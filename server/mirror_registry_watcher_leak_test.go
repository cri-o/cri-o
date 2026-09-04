package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	kclock "k8s.io/utils/clock"
)

type fakeMirrorRegistryTimer struct {
	ticks   chan time.Time
	resets  chan time.Duration
	stopped chan struct{}
	stop    sync.Once
}

func newFakeMirrorRegistryTimer() *fakeMirrorRegistryTimer {
	return &fakeMirrorRegistryTimer{
		ticks:   make(chan time.Time, 1),
		resets:  make(chan time.Duration, 1),
		stopped: make(chan struct{}),
	}
}

func (t *fakeMirrorRegistryTimer) C() <-chan time.Time {
	return t.ticks
}

func (t *fakeMirrorRegistryTimer) Stop() bool {
	t.stop.Do(func() {
		close(t.stopped)
	})

	return true
}

func (t *fakeMirrorRegistryTimer) Reset(d time.Duration) bool {
	t.resets <- d

	return true
}

func receiveMirrorWatcherValue[T any](t *testing.T, values <-chan T) T {
	t.Helper()

	select {
	case value := <-values:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for mirror-registry watcher")
	}

	var zero T

	return zero
}

func requireMirrorWatcherStopped(t *testing.T, done <-chan struct{}) {
	t.Helper()
	receiveMirrorWatcherValue(t, done)
}

func startMirrorRegistryEventLoop(
	t *testing.T,
	ctx context.Context,
	events <-chan fsnotify.Event,
	reload func(string),
) (
	timer *fakeMirrorRegistryTimer,
	timerStarted <-chan time.Duration,
	done <-chan struct{},
) {
	t.Helper()

	timer = newFakeMirrorRegistryTimer()
	timerStartedChannel := make(chan time.Duration, 1)
	doneChannel := make(chan struct{})

	go func() {
		watchAndReloadMirrorRegistriesConfiguration(
			ctx,
			events,
			make(chan error),
			reload,
			func(d time.Duration) kclock.Timer {
				timerStartedChannel <- d

				return timer
			},
		)
		close(doneChannel)
	}()

	return timer, timerStartedChannel, doneChannel
}

func TestMirrorRegistryWatcherStop(t *testing.T) {
	s := &Server{}
	s.startMirrorRegistryWatcher(context.Background(), t.TempDir())

	done := make(chan struct{})

	go func() {
		s.stopMirrorRegistryWatcher()
		close(done)
	}()

	requireMirrorWatcherStopped(t, done)

	// Stopping an already stopped watcher is safe.
	s.stopMirrorRegistryWatcher()
}

func TestWatchAndReloadMirrorRegistriesConfiguration(t *testing.T) {
	t.Run("debounces events", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		events := make(chan fsnotify.Event, 2)
		reloads := make(chan string, 1)
		timer, timerStarted, done := startMirrorRegistryEventLoop(t, ctx, events, func(name string) {
			reloads <- name
		})

		events <- fsnotify.Event{Name: "first.conf", Op: fsnotify.Write}

		if d := receiveMirrorWatcherValue(t, timerStarted); d != debounceDuration {
			t.Fatalf("expected debounce duration %s, got %s", debounceDuration, d)
		}

		events <- fsnotify.Event{Name: "second.conf", Op: fsnotify.Write}

		if d := receiveMirrorWatcherValue(t, timer.resets); d != debounceDuration {
			t.Fatalf("expected reset duration %s, got %s", debounceDuration, d)
		}

		timer.ticks <- time.Now()

		if name := receiveMirrorWatcherValue(t, reloads); name != "second.conf" {
			t.Fatalf("expected latest event to trigger reload, got %q", name)
		}

		select {
		case name := <-reloads:
			t.Fatalf("expected events to be debounced, got extra reload for %q", name)
		default:
		}

		cancel()
		requireMirrorWatcherStopped(t, done)
	})

	t.Run("cancels pending reload", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		events := make(chan fsnotify.Event, 1)
		reloads := make(chan string, 1)
		timer, timerStarted, done := startMirrorRegistryEventLoop(t, ctx, events, func(name string) {
			reloads <- name
		})

		events <- fsnotify.Event{Name: "pending.conf", Op: fsnotify.Write}

		receiveMirrorWatcherValue(t, timerStarted)
		cancel()
		requireMirrorWatcherStopped(t, done)

		select {
		case <-timer.stopped:
		default:
			t.Fatal("pending debounce timer was not stopped")
		}

		timer.ticks <- time.Now()

		select {
		case name := <-reloads:
			t.Fatalf("unexpected reload after cancellation for %q", name)
		default:
		}
	})

	t.Run("joins active reload", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		events := make(chan fsnotify.Event, 1)
		reloadStarted := make(chan struct{})
		releaseReload := make(chan struct{})
		timer, timerStarted, done := startMirrorRegistryEventLoop(t, ctx, events, func(string) {
			close(reloadStarted)
			<-releaseReload
		})

		events <- fsnotify.Event{Name: "active.conf", Op: fsnotify.Write}

		receiveMirrorWatcherValue(t, timerStarted)

		timer.ticks <- time.Now()

		receiveMirrorWatcherValue(t, reloadStarted)
		cancel()

		select {
		case <-done:
			t.Fatal("watcher stopped before active reload completed")
		default:
		}

		close(releaseReload)
		requireMirrorWatcherStopped(t, done)
	})
}
