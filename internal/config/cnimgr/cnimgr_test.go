package cnimgr

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cri-o/ocicni/pkg/ocicni"
)

const (
	testPollInterval = 10 * time.Millisecond
	testTimeout      = 2 * time.Second
)

type fakeCNIPlugin struct {
	mu        sync.Mutex
	statusErr error
	gcCalls   atomic.Int32
}

func (f *fakeCNIPlugin) setStatusErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.statusErr = err
}

func (f *fakeCNIPlugin) Status() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.statusErr
}

func (f *fakeCNIPlugin) Name() string                  { return "fake" }
func (f *fakeCNIPlugin) GetDefaultNetworkName() string { return "fake-net" }
func (f *fakeCNIPlugin) Shutdown() error               { return nil }

func (f *fakeCNIPlugin) GC(context.Context, []*ocicni.PodNetwork) error {
	f.gcCalls.Add(1)

	return nil
}

func (f *fakeCNIPlugin) StatusWithContext(context.Context) error { return f.Status() }

func (f *fakeCNIPlugin) SetUpPod(ocicni.PodNetwork) ([]ocicni.NetResult, error) {
	return nil, nil
}

func (f *fakeCNIPlugin) SetUpPodWithContext(context.Context, ocicni.PodNetwork) ([]ocicni.NetResult, error) {
	return nil, nil
}

func (f *fakeCNIPlugin) TearDownPod(ocicni.PodNetwork) error { return nil }

func (f *fakeCNIPlugin) TearDownPodWithContext(context.Context, ocicni.PodNetwork) error {
	return nil
}

func (f *fakeCNIPlugin) GetPodNetworkStatus(ocicni.PodNetwork) ([]ocicni.NetResult, error) {
	return nil, nil
}

func (f *fakeCNIPlugin) GetPodNetworkStatusWithContext(context.Context, ocicni.PodNetwork) ([]ocicni.NetResult, error) {
	return nil, nil
}

func newTestManager(plugin *fakeCNIPlugin) *CNIManager {
	ctx, cancel := context.WithCancel(context.Background())

	mgr := &CNIManager{
		plugin:           plugin,
		lastError:        errors.New("plugin status uninitialized"),
		cancel:           cancel,
		initPollInterval: testPollInterval,
	}

	go mgr.pollUntilReady(ctx)

	return mgr
}

func waitFor(t *testing.T, desc string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(testTimeout)

	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for: %s", desc)
}

func TestStatusPolling(t *testing.T) {
	t.Run("becomes ready when plugin is healthy", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})
	})

	t.Run("stays not-ready while plugin is unhealthy", func(t *testing.T) {
		fake := &fakeCNIPlugin{statusErr: errors.New("not yet")}

		mgr := newTestManager(fake)
		defer mgr.Shutdown()

		time.Sleep(100 * time.Millisecond)

		if err := mgr.ReadyOrError(); err == nil {
			t.Fatal("expected not-ready")
		}
	})

	t.Run("becomes ready after plugin recovers at startup", func(t *testing.T) {
		fake := &fakeCNIPlugin{statusErr: errors.New("not yet")}

		mgr := newTestManager(fake)
		defer mgr.Shutdown()

		fake.setStatusErr(nil)

		waitFor(t, "ready after startup recovery", func() bool {
			return mgr.ReadyOrError() == nil
		})
	})

	t.Run("shutdown stops polling", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		mgr.Shutdown()

		if !errors.Is(mgr.ReadyOrError(), errShutdown) {
			t.Fatalf("expected shutdown error, got: %v", mgr.ReadyOrError())
		}
	})

	t.Run("ReadyOrError returns error after shutdown", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		mgr.Shutdown()

		if err := mgr.ReadyOrError(); err == nil {
			t.Fatal("expected error after shutdown")
		}
	})

	t.Run("watcher notified on initial readiness", func(t *testing.T) {
		fake := &fakeCNIPlugin{statusErr: errors.New("not yet")}

		mgr := newTestManager(fake)
		defer mgr.Shutdown()

		watcher := mgr.AddWatcher()

		fake.setStatusErr(nil)

		select {
		case ready := <-watcher:
			if !ready {
				t.Fatal("expected watcher to receive true")
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out waiting for watcher")
		}
	})

	t.Run("watcher receives true immediately if already ready", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		watcher := mgr.AddWatcher()

		select {
		case ready := <-watcher:
			if !ready {
				t.Fatal("expected watcher to receive true immediately")
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out waiting for immediate watcher notification")
		}
	})

	t.Run("watcher receives false on shutdown", func(t *testing.T) {
		fake := &fakeCNIPlugin{statusErr: errors.New("not yet")}
		mgr := newTestManager(fake)

		watcher := mgr.AddWatcher()

		go func() {
			time.Sleep(50 * time.Millisecond)
			mgr.Shutdown()
		}()

		select {
		case ready := <-watcher:
			if ready {
				t.Fatal("expected watcher to receive false on shutdown")
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out waiting for shutdown notification")
		}
	})

	t.Run("AddWatcher returns false after shutdown", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		mgr.Shutdown()

		watcher := mgr.AddWatcher()

		select {
		case ready := <-watcher:
			if ready {
				t.Fatal("expected watcher to receive false after shutdown")
			}
		case <-time.After(testTimeout):
			t.Fatal("timed out waiting for watcher after shutdown")
		}
	})

	t.Run("GC called on initial readiness", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)
		mgr.mutex.Lock()
		mgr.validPodList = func() ([]*ocicni.PodNetwork, error) { return nil, nil }
		mgr.mutex.Unlock()

		defer mgr.Shutdown()

		waitFor(t, "GC called", func() bool {
			return fake.gcCalls.Load() > 0
		})
	})

	t.Run("GC not called while already healthy", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)
		mgr.mutex.Lock()
		mgr.validPodList = func() ([]*ocicni.PodNetwork, error) { return nil, nil }
		mgr.mutex.Unlock()

		defer mgr.Shutdown()

		waitFor(t, "initial ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		initialGC := fake.gcCalls.Load()

		time.Sleep(200 * time.Millisecond)

		if calls := fake.gcCalls.Load(); calls != initialGC {
			t.Fatalf("expected no additional GC calls while healthy, got %d (initial was %d)", calls, initialGC)
		}
	})

	t.Run("Plugin returns the CNI plugin", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)
		defer mgr.Shutdown()

		if mgr.Plugin() == nil {
			t.Fatal("expected non-nil plugin")
		}
	})

	t.Run("GC deferred when plugin is not ready", func(t *testing.T) {
		fake := &fakeCNIPlugin{statusErr: errors.New("not yet")}

		mgr := newTestManager(fake)
		defer mgr.Shutdown()

		err := mgr.GC(context.Background(), func() ([]*ocicni.PodNetwork, error) {
			return nil, nil
		})
		if err != nil {
			t.Fatalf("expected nil error when GC is deferred, got: %v", err)
		}

		if calls := fake.gcCalls.Load(); calls != 0 {
			t.Fatalf("expected 0 GC calls while not ready, got %d", calls)
		}
	})

	t.Run("GC runs immediately when plugin is ready", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		initialGC := fake.gcCalls.Load()

		err := mgr.GC(context.Background(), func() ([]*ocicni.PodNetwork, error) {
			return nil, nil
		})
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}

		if calls := fake.gcCalls.Load(); calls <= initialGC {
			t.Fatalf("expected GC to run immediately, got %d calls (initial was %d)", calls, initialGC)
		}
	})

	t.Run("GC returns error from validPodList", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		expectedErr := errors.New("pod list error")

		err := mgr.GC(context.Background(), func() ([]*ocicni.PodNetwork, error) {
			return nil, expectedErr
		})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected pod list error, got: %v", err)
		}
	})
}

func newTestManagerWithMonitoring(plugin *fakeCNIPlugin) *CNIManager {
	return newTestManagerWithMonitoringAndGrace(plugin, 0)
}

func newTestManagerWithMonitoringAndGrace(plugin *fakeCNIPlugin, gracePeriod time.Duration) *CNIManager {
	ctx, cancel := context.WithCancel(context.Background())

	mgr := &CNIManager{
		plugin:              plugin,
		lastError:           errors.New("plugin status uninitialized"),
		cancel:              cancel,
		initPollInterval:    testPollInterval,
		monitorPollInterval: testPollInterval,
		monitoringEnabled:   true,
		gracePeriod:         gracePeriod,
	}

	go mgr.pollContinuously(ctx)

	return mgr
}

func TestContinuousMonitoring(t *testing.T) {
	t.Run("detects plugin failure after startup", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManagerWithMonitoring(fake)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		fake.setStatusErr(errors.New("plugin crashed"))

		waitFor(t, "not-ready via monitoring", func() bool {
			return mgr.ReadyOrError() != nil
		})
	})

	t.Run("self-heals after recovery", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManagerWithMonitoring(fake)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		fake.setStatusErr(errors.New("plugin crashed"))

		waitFor(t, "not-ready", func() bool {
			return mgr.ReadyOrError() != nil
		})

		fake.setStatusErr(nil)

		waitFor(t, "recovered", func() bool {
			return mgr.ReadyOrError() == nil
		})
	})

	t.Run("monitoring disabled does not detect post-startup failure", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManager(fake) // monitoring disabled
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		fake.setStatusErr(errors.New("plugin crashed"))

		// Give Phase 2 time to run (it shouldn't be running)
		time.Sleep(100 * time.Millisecond)

		if mgr.ReadyOrError() != nil {
			t.Fatal("expected ReadyOrError to remain nil when monitoring is disabled")
		}
	})

	t.Run("GC not triggered on Phase 2 recovery", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManagerWithMonitoring(fake)
		mgr.mutex.Lock()
		mgr.validPodList = func() ([]*ocicni.PodNetwork, error) { return nil, nil }
		mgr.mutex.Unlock()

		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		gcBefore := fake.gcCalls.Load()

		fake.setStatusErr(errors.New("plugin crashed"))

		waitFor(t, "not-ready", func() bool {
			return mgr.ReadyOrError() != nil
		})

		fake.setStatusErr(nil)

		waitFor(t, "recovered", func() bool {
			return mgr.ReadyOrError() == nil
		})

		if calls := fake.gcCalls.Load(); calls != gcBefore {
			t.Fatalf("expected no GC calls on Phase 2 recovery, got %d additional calls", calls-gcBefore)
		}
	})
}

func TestGracePeriod(t *testing.T) {
	t.Run("delays unhealthy reporting until grace period expires", func(t *testing.T) {
		fake := &fakeCNIPlugin{}
		gracePeriod := 200 * time.Millisecond

		mgr := newTestManagerWithMonitoringAndGrace(fake, gracePeriod)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		fake.setStatusErr(errors.New("plugin crashed"))

		// Within grace period, should still report healthy
		time.Sleep(50 * time.Millisecond)

		if mgr.ReadyOrError() != nil {
			t.Fatal("expected ReadyOrError to remain nil within grace period")
		}

		// After grace period expires, should report unhealthy
		waitFor(t, "unhealthy after grace", func() bool {
			return mgr.ReadyOrError() != nil
		})
	})

	t.Run("plugin recovers within grace period stays healthy", func(t *testing.T) {
		fake := &fakeCNIPlugin{}
		gracePeriod := 500 * time.Millisecond

		mgr := newTestManagerWithMonitoringAndGrace(fake, gracePeriod)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		fake.setStatusErr(errors.New("brief disruption"))

		// Wait long enough for several poll cycles but within grace period
		time.Sleep(100 * time.Millisecond)

		if mgr.ReadyOrError() != nil {
			t.Fatal("expected ReadyOrError to remain nil during brief disruption")
		}

		fake.setStatusErr(nil)

		// Give a few poll cycles for recovery to register
		time.Sleep(50 * time.Millisecond)

		if mgr.ReadyOrError() != nil {
			t.Fatal("expected ReadyOrError to remain nil after recovery within grace period")
		}
	})

	t.Run("zero grace period reports unhealthy immediately", func(t *testing.T) {
		fake := &fakeCNIPlugin{}

		mgr := newTestManagerWithMonitoringAndGrace(fake, 0)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		fake.setStatusErr(errors.New("plugin crashed"))

		waitFor(t, "immediately unhealthy", func() bool {
			return mgr.ReadyOrError() != nil
		})
	})

	t.Run("grace period timer resets after full recovery", func(t *testing.T) {
		fake := &fakeCNIPlugin{}
		gracePeriod := 200 * time.Millisecond

		mgr := newTestManagerWithMonitoringAndGrace(fake, gracePeriod)
		defer mgr.Shutdown()

		waitFor(t, "ready", func() bool {
			return mgr.ReadyOrError() == nil
		})

		// First failure: let grace period expire
		fake.setStatusErr(errors.New("first failure"))

		waitFor(t, "unhealthy after first grace", func() bool {
			return mgr.ReadyOrError() != nil
		})

		// Recover
		fake.setStatusErr(nil)

		waitFor(t, "recovered", func() bool {
			return mgr.ReadyOrError() == nil
		})

		// Second failure: grace period should apply again from scratch
		fake.setStatusErr(errors.New("second failure"))

		// Within grace period, should still be healthy
		time.Sleep(50 * time.Millisecond)

		if mgr.ReadyOrError() != nil {
			t.Fatal("expected grace period to reset after recovery")
		}

		// After grace period, should report unhealthy
		waitFor(t, "unhealthy after second grace", func() bool {
			return mgr.ReadyOrError() != nil
		})
	})
}
