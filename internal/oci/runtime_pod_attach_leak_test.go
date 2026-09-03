package oci

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	conmonClient "github.com/containers/conmon-rs/pkg/client"
	"go.podman.io/common/pkg/resize"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/cri-streaming/pkg/streaming/remotecommand"
)

// countVendoredReceivers counts goroutines blocked inside the vendored
// resize.HandleResizing helper used by conmon-rs. It is a secondary,
// defense-in-depth signal; the primary leak oracle is requireDownstreamClosed,
// which deterministically checks the ownership contract.
func countVendoredReceivers() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)

	count := 0

	for s := range strings.SplitSeq(string(buf[:n]), "\n\n") {
		if strings.Contains(s, "HandleResizing") && strings.Contains(s, "chan receive") {
			count++
		}
	}

	return count
}

// waitFor polls cond until true or the timeout expires, failing the test.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

// requireDownstreamClosed drains any buffered sizes and requires the
// downstream channel to be closed within the timeout. A non-closed channel
// is exactly the leaked state: the vendored receiver can never exit.
func requireDownstreamClosed(t *testing.T, ch <-chan resize.TerminalSize, what string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		default:
		}

		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for downstream to close (%s)", what)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

// newTestPodContainer builds a minimal container for attach tests.
func newTestPodContainer(t *testing.T, id string, stdinOnce bool) *Container {
	t.Helper()

	c, err := NewContainer(id, "name", t.TempDir(), "logPath",
		map[string]string{}, map[string]string{}, map[string]string{},
		"image", nil, nil, "", &types.ContainerMetadata{}, "sandbox",
		false, true, stdinOnce, "", t.TempDir(), time.Now(), "")
	if err != nil {
		t.Fatalf("create test container: %v", err)
	}

	return c
}

// attachCapture stubs the conmon attach RPC. It starts the vendored resize
// receiver exactly like conmon-rs does before dialing, records the downstream
// channel it was given, signals once the receiver is installed, and then
// blocks until released or fails with err, mimicking attach duration and
// dial failure respectively.
type attachCapture struct {
	mu         sync.Mutex
	downstream []<-chan resize.TerminalSize
	started    chan struct{}
	release    chan struct{}
	onSize     func(resize.TerminalSize)
	err        error

	once sync.Once
}

// newAttachCapture returns a stub that reports sizes to onSize (or drops
// them) and fails with err when set, mimicking dial failure.
func newAttachCapture(onSize func(resize.TerminalSize), err error) *attachCapture {
	if onSize == nil {
		onSize = func(resize.TerminalSize) {}
	}

	return &attachCapture{
		started: make(chan struct{}),
		release: make(chan struct{}),
		onSize:  onSize,
		err:     err,
	}
}

// stub returns an attachFunc that installs the vendored receiver, records
// the downstream channel, and blocks until released or fails immediately.
func (a *attachCapture) stub() func(context.Context, *conmonClient.AttachConfig) error {
	return func(_ context.Context, cfg *conmonClient.AttachConfig) error {
		a.mu.Lock()
		a.downstream = append(a.downstream, cfg.Resize)
		a.mu.Unlock()

		resize.HandleResizing(cfg.Resize, a.onSize)
		a.once.Do(func() { close(a.started) })

		if a.err != nil {
			return a.err
		}

		<-a.release

		return nil
	}
}

// releaseAll unblocks a stub waiting in stub, exactly once.
func (a *attachCapture) releaseAll() {
	select {
	case <-a.release:
	default:
		close(a.release)
	}
}

// requireAllClosed requires every downstream channel the stub observed to
// be closed, proving AttachContainer released conmon's receiver.
func (a *attachCapture) requireAllClosed(t *testing.T, what string) {
	t.Helper()

	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.downstream) == 0 {
		t.Fatalf("stub never ran (%s)", what)
	}

	for i, ch := range a.downstream {
		requireDownstreamClosed(t, ch, what)
		_ = i
	}
}

// TestRuntimePodAttachContainerNoStrandedReceiver is a regression test for a
// goroutine leak in pod attach: AttachContainer allocated the downstream
// resize channel but never closed it, so the vendored resize receiver started
// by conmon-rs blocked permanently. It drives the real AttachContainer with
// a stubbed conmon attach (no live server needed). The primary oracle
// requires each downstream channel to be closed; receiver stack scans are
// secondary.
func TestRuntimePodAttachContainerNoStrandedReceiver(t *testing.T) {
	t.Run("nil upstream non-TTY", func(t *testing.T) {
		before := countVendoredReceivers()
		cap := newAttachCapture(nil, nil)

		rp := &runtimePod{serverDir: t.TempDir(), attachFunc: cap.stub()}
		c := newTestPodContainer(t, "pod-attach-leak-nil", false)

		done := make(chan error, 1)
		go func() {
			done <- rp.AttachContainer(context.Background(), c, nil, nil, nil, false, nil)
		}()

		select {
		case <-cap.started:
		case <-time.After(10 * time.Second):
			t.Fatal("stub never started")
		}

		cap.releaseAll()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("AttachContainer returned error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("AttachContainer did not return")
		}

		cap.requireAllClosed(t, "nil upstream")

		waitFor(t, 10*time.Second, "vendored receiver to exit", func() bool {
			return countVendoredReceivers()-before == 0
		})
	})

	t.Run("TTY forwards then releases with upstream open", func(t *testing.T) {
		before := countVendoredReceivers()

		upstream := make(chan remotecommand.TerminalSize, 2)
		t.Cleanup(func() {
			select {
			case <-upstream:
			default:
			}
		})

		received := make(chan resize.TerminalSize, 2)
		cap := newAttachCapture(func(s resize.TerminalSize) {
			received <- s
		}, nil)

		rp := &runtimePod{serverDir: t.TempDir(), attachFunc: cap.stub()}
		c := newTestPodContainer(t, "pod-attach-leak-tty", false)

		done := make(chan error, 1)
		go func() {
			done <- rp.AttachContainer(context.Background(), c, nil, nil, nil, true, upstream)
		}()

		select {
		case <-cap.started:
		case <-time.After(10 * time.Second):
			t.Fatal("stub never started")
		}

		upstream <- remotecommand.TerminalSize{Height: 24, Width: 80}

		select {
		case got := <-received:
			if got.Height != 24 || got.Width != 80 {
				t.Fatalf("forwarded size = %+v, want 24x80", got)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for forwarded resize event")
		}

		// Release attach while upstream is still open: cleanup must stop
		// the bridge even though no upstream close arrived.
		cap.releaseAll()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("AttachContainer returned error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("AttachContainer did not return")
		}

		cap.requireAllClosed(t, "TTY")
		// AttachContainer only returns after cleanup joins the bridge, so a
		// return already proves bridge termination with upstream open.

		waitFor(t, 10*time.Second, "vendored receiver to exit", func() bool {
			return countVendoredReceivers()-before == 0
		})
	})

	t.Run("dial failure still releases", func(t *testing.T) {
		before := countVendoredReceivers()

		dialErr := errors.New("failed to connect to container's attach socket")
		cap := newAttachCapture(nil, dialErr)

		rp := &runtimePod{serverDir: t.TempDir(), attachFunc: cap.stub()}
		c := newTestPodContainer(t, "pod-attach-leak-dial", false)

		upstream := make(chan remotecommand.TerminalSize)
		defer close(upstream)

		if err := rp.AttachContainer(context.Background(), c, nil, nil, nil, true, upstream); !errors.Is(err, dialErr) {
			t.Fatalf("AttachContainer error = %v, want %v", err, dialErr)
		}

		cap.requireAllClosed(t, "dial failure")

		waitFor(t, 10*time.Second, "vendored receiver to exit after dial failure", func() bool {
			return countVendoredReceivers()-before == 0
		})
	})

	t.Run("cancelled context still releases", func(t *testing.T) {
		before := countVendoredReceivers()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		cap := newAttachCapture(nil, ctx.Err())

		rp := &runtimePod{serverDir: t.TempDir(), attachFunc: cap.stub()}
		c := newTestPodContainer(t, "pod-attach-leak-cancel", false)

		if err := rp.AttachContainer(ctx, c, nil, nil, nil, false, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("AttachContainer error = %v, want context.Canceled", err)
		}

		cap.requireAllClosed(t, "cancelled context")

		waitFor(t, 10*time.Second, "vendored receiver to exit after cancel", func() bool {
			return countVendoredReceivers()-before == 0
		})
	})

	t.Run("leave-stdin-open flag propagates and releases", func(t *testing.T) {
		before := countVendoredReceivers()

		var gotEOF bool

		cap := newAttachCapture(nil, nil)
		orig := cap.stub()
		rp := &runtimePod{
			serverDir: t.TempDir(),
			attachFunc: func(ctx context.Context, cfg *conmonClient.AttachConfig) error {
				gotEOF = cfg.StopAfterStdinEOF
				return orig(ctx, cfg)
			},
		}
		// stdin=true, stdinOnce=true, non-TTY => StopAfterStdinEOF must be true.
		c := newTestPodContainer(t, "pod-attach-leak-eof", true)

		done := make(chan error, 1)
		go func() {
			done <- rp.AttachContainer(context.Background(), c, nil, nil, nil, false, nil)
		}()

		select {
		case <-cap.started:
		case <-time.After(10 * time.Second):
			t.Fatal("stub never started")
		}

		cap.releaseAll()

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("AttachContainer returned error: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("AttachContainer did not return")
		}

		if !gotEOF {
			t.Fatal("StopAfterStdinEOF = false, want true for stdinOnce container")
		}

		cap.requireAllClosed(t, "stdin-once")
		_ = before

		waitFor(t, 10*time.Second, "vendored receiver to exit", func() bool {
			return countVendoredReceivers()-before == 0
		})
	})

	t.Run("repeated sessions do not accumulate", func(t *testing.T) {
		before := countVendoredReceivers()

		var all []*attachCapture

		for range 20 {
			cap := newAttachCapture(nil, nil)
			all = append(all, cap)

			rp := &runtimePod{serverDir: t.TempDir(), attachFunc: cap.stub()}
			c := newTestPodContainer(t, "pod-attach-leak-repeat", false)

			done := make(chan error, 1)
			go func() {
				done <- rp.AttachContainer(context.Background(), c, nil, nil, nil, false, nil)
			}()

			select {
			case <-cap.started:
			case <-time.After(10 * time.Second):
				t.Fatal("stub never started")
			}

			cap.releaseAll()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("AttachContainer returned error: %v", err)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("AttachContainer did not return")
			}
		}

		for _, cap := range all {
			cap.requireAllClosed(t, "repeated sessions")
		}

		waitFor(t, 10*time.Second, "repeated receivers to exit", func() bool {
			return countVendoredReceivers()-before == 0
		})
	})
}
