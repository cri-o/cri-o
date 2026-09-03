package server

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cri-o/cri-o/internal/signals"
)

// reloadWatcherWorkers counts in-flight serveReloadNotifications goroutines
// by function name rather than closure number, which shifts when unrelated
// literals are added. The stack buffer grows instead of accepting a
// truncated dump.
func reloadWatcherWorkers() int {
	bufSize := 1 << 20

	for {
		buf := make([]byte, bufSize)

		n := runtime.Stack(buf, true)
		if n == len(buf) {
			bufSize *= 2

			continue
		}

		workers := 0

		for stack := range strings.SplitSeq(string(buf[:n]), "\n\n") {
			if strings.Contains(stack, "serveReloadNotifications.func") {
				workers++
			}
		}

		return workers
	}
}

func waitForReloadWatcherCount(want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if reloadWatcherWorkers() == want {
			return true
		}

		time.Sleep(10 * time.Millisecond)
	}

	return reloadWatcherWorkers() == want
}

func TestStopReloadWatcherExitsWithoutSignal(t *testing.T) {
	s := &Server{}
	before := reloadWatcherWorkers()

	s.startReloadWatcher(context.Background())

	if !waitForReloadWatcherCount(before+1, 5*time.Second) {
		t.Fatalf("reload watcher goroutine did not start (before=%d after=%d)", before, reloadWatcherWorkers())
	}

	// No SIGHUP is sent: the stop signal alone must release the watcher.
	s.stopReloadWatcher()

	if !waitForReloadWatcherCount(before, 5*time.Second) {
		t.Fatalf("reload watcher goroutine did not exit after stop (before=%d after=%d)", before, reloadWatcherWorkers())
	}
}

func TestStopReloadWatcherIdempotentAndNilSafe(t *testing.T) {
	baseline := reloadWatcherWorkers()

	s := &Server{}
	s.startReloadWatcher(context.Background())

	if !waitForReloadWatcherCount(baseline+1, 5*time.Second) {
		t.Fatalf("expected reload watcher to be running (baseline=%d after=%d)", baseline, reloadWatcherWorkers())
	}

	s.stopReloadWatcher()
	// Second stop must not block or panic once stopped is closed.
	s.stopReloadWatcher()

	if !waitForReloadWatcherCount(baseline, 5*time.Second) {
		t.Fatalf("reload watcher did not exit: baseline=%d after=%d", baseline, reloadWatcherWorkers())
	}

	// Never-started and nil servers must be safe to stop.
	(&Server{}).stopReloadWatcher()

	var nilServer *Server

	nilServer.stopReloadWatcher()
}

func TestReloadWatcherRepeatedLifecyclesDoNotAccumulate(t *testing.T) {
	before := reloadWatcherWorkers()

	for range 5 {
		s := &Server{}
		s.startReloadWatcher(context.Background())
		s.stopReloadWatcher()
	}

	if !waitForReloadWatcherCount(before, 5*time.Second) {
		t.Fatalf("repeated lifecycles accumulated watchers (before=%d after=%d)", before, reloadWatcherWorkers())
	}
}

func TestReloadWatcherDeliversHUPToReload(t *testing.T) {
	ctx := t.Context()

	reloaded := make(chan struct{}, 1)

	// An error return keeps the handler on its log-and-continue path, so no
	// production post-reload work (metrics, image server, config dump) runs
	// against this zero-value Server.
	reload := func(context.Context) error {
		select {
		case reloaded <- struct{}{}:
		default:
		}

		return errors.New("stub reload failure")
	}

	s := &Server{}

	// Synthetic channel: proves HUP wiring without signaling the test process.
	ch := make(chan os.Signal, 1)

	s.serveReloadNotifications(ctx, ch, reload)

	// The stub must not fire before any notification arrives.
	select {
	case <-reloaded:
		s.stopReloadWatcher()

		t.Fatal("reload invoked without any HUP notification")
	case <-time.After(100 * time.Millisecond):
	}

	before := reloadWatcherWorkers()

	ch <- signals.Hup

	select {
	case <-reloaded:
	case <-time.After(5 * time.Second):
		s.stopReloadWatcher()

		t.Fatal("synthetic SIGHUP did not invoke reload within 5s")
	}

	// The error-returning stub sends the handler back to its select, so the
	// watcher must still be alive until explicitly stopped.
	if !waitForReloadWatcherCount(before, time.Second) {
		s.stopReloadWatcher()

		t.Fatalf("watcher exited after handled HUP (workers=%d, want %d)", reloadWatcherWorkers(), before)
	}

	s.stopReloadWatcher()

	if !waitForReloadWatcherCount(before-1, 5*time.Second) {
		t.Fatalf("watcher did not exit after stop (workers=%d, want %d)", reloadWatcherWorkers(), before-1)
	}
}

func TestReloadWatcherExitsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Server{}
	before := reloadWatcherWorkers()

	s.startReloadWatcher(ctx)

	if !waitForReloadWatcherCount(before+1, 5*time.Second) {
		cancel()

		t.Fatalf("reload watcher goroutine did not start (before=%d after=%d)", before, reloadWatcherWorkers())
	}

	cancel()

	if !waitForReloadWatcherCount(before, 5*time.Second) {
		// Join explicitly so the test never leaves a stray watcher behind.
		s.stopReloadWatcher()

		t.Fatalf("reload watcher did not exit on context cancel (before=%d after=%d)", before, reloadWatcherWorkers())
	}

	// Join explicitly; stopped is already closed so this returns immediately.
	s.stopReloadWatcher()
}
