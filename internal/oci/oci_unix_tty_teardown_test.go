//go:build !windows

package oci

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// gatedStream models a backpressured SPDY stream: reads block until Reset
// (like Stream.Read selecting on closeChan), writes block until Reset (like a
// data write stalled on the framer lock / transport backpressure), and Close
// blocks while backpressured (like a FIN write contending for the same lock).
// Reset releases local waiters immediately, mirroring closeRemoteChannels
// running before the RST frame write.
type gatedStream struct {
	done       chan struct{}
	once       sync.Once
	resetCalls atomic.Int32
}

func newGatedStream() *gatedStream {
	return &gatedStream{done: make(chan struct{})}
}

func (g *gatedStream) Reset() error {
	g.resetCalls.Add(1)
	g.once.Do(func() { close(g.done) })

	return nil
}

func (g *gatedStream) resetDone() <-chan struct{} {
	return g.done
}

type gatedStdin struct {
	*gatedStream
}

func (s *gatedStdin) Read(p []byte) (int, error) {
	select {
	case <-s.resetDone():
		return 0, io.EOF
	case <-time.After(30 * time.Second):
		return 0, errors.New("test timeout: stdin Read was never reset")
	}
}

type gatedStdout struct {
	*gatedStream
}

func (s *gatedStdout) Write(p []byte) (int, error) {
	select {
	case <-s.resetDone():
		return 0, errors.New("stream reset")
	case <-time.After(30 * time.Second):
		return 0, errors.New("test timeout: stdout Write was never reset")
	}
}

func (s *gatedStdout) Close() error {
	select {
	case <-s.resetDone():
		return nil
	case <-time.After(30 * time.Second):
		return nil
	}
}

func newTTYTestContainer(t *testing.T) *Container {
	t.Helper()

	c, err := NewContainer("tty-teardown-test", "name", t.TempDir(), "logPath",
		map[string]string{}, map[string]string{}, map[string]string{},
		"image", nil, nil, "", &types.ContainerMetadata{}, "sandbox",
		false, false, false, "", t.TempDir(), time.Now(), "")
	if err != nil {
		t.Fatalf("create test container: %v", err)
	}

	return c
}

// TestTTYCmdTeardownUnderOutputBackpressure reproduces A4: the command exits
// while the stdin copier is parked in Read and the stdout copier is parked in
// a backpressured Write. Without the fix, the deferred graceful stdout.Close
// blocks behind the stalled write, ttyCmd never returns, and ServeExec's
// deferred connection reset is unreachable. The fix must reset the
// connection-owned stdin stream and bound the output close so ttyCmd returns.
func TestTTYCmdTeardownUnderOutputBackpressure(t *testing.T) {
	stdin := &gatedStdin{gatedStream: newGatedStream()}
	stdout := &gatedStdout{gatedStream: newGatedStream()}

	// Exits immediately with finite output; teardown must not wait for the
	// gated peer.
	execCmd := exec.Command("sh", "-c", "echo hi; exit 0")

	done := make(chan error, 1)
	go func() {
		done <- ttyCmd(execCmd, stdin, stdout, nil, newTTYTestContainer(t))
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ttyCmd returned error: %v", err)
		}
	case <-time.After(25 * time.Second):
		t.Fatal("ttyCmd did not return within 25s under output backpressure (teardown cycle)")
	}

	if stdin.resetCalls.Load() == 0 {
		t.Fatal("command completion did not reset the stdin stream")
	}
}

// TestTTYCmdHappyPathPreservesOutput guards the normal path: with a draining
// peer, output is still copied and the command result returned.
func TestTTYCmdHappyPathPreservesOutput(t *testing.T) {
	stdin := strings.NewReader("")
	var stdout bytes.Buffer

	execCmd := exec.Command("sh", "-c", "echo hello; exit 0")

	done := make(chan error, 1)
	go func() {
		done <- ttyCmd(execCmd, stdin, discardTTYCloser{Writer: &stdout}, nil, newTTYTestContainer(t))
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ttyCmd returned error: %v", err)
		}
	case <-time.After(25 * time.Second):
		t.Fatal("ttyCmd did not return within 25s on happy path")
	}

	if !strings.Contains(stdout.String(), "hello") {
		t.Fatalf("expected PTY output to contain %q, got %q", "hello", stdout.String())
	}
}

type discardTTYCloser struct {
	io.Writer
}

// discardTTYCloser collects PTY output without observing close signals.
func (discardTTYCloser) Close() error { return nil }
