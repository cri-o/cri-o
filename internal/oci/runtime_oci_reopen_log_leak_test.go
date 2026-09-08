package oci

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// newReopenTestContainer builds a minimal container pointing at bundleDir
// and logPath for ReopenContainerLog tests.
func newReopenTestContainer(t *testing.T, bundleDir, logPath string) *Container {
	t.Helper()

	c, err := NewContainer("reopen-leak-id", "name", bundleDir, logPath,
		map[string]string{}, map[string]string{}, map[string]string{},
		"image", nil, nil, "", &types.ContainerMetadata{}, "sandbox",
		false, false, false, "", t.TempDir(), time.Now(), "")
	if err != nil {
		t.Fatalf("create test container: %v", err)
	}

	return c
}

// writeCtl creates the bundle ctl file ReopenContainerLog opens.
func writeCtl(t *testing.T, bundleDir string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(bundleDir, "ctl"), []byte{}, 0o644); err != nil {
		t.Fatalf("write ctl file: %v", err)
	}
}

// TestRuntimeOCIReopenContainerLogWatchFailureIsBestEffort is a regression
// test for the reopen-watcher leak: the watcher goroutine was launched before
// watcher.Add, so a missing log directory closed done, returned nil, closed
// the fsnotify channels, and stranded the worker on an unbuffered errorCh
// send. The Add failure must stay best effort (nil error, reopen command
// still sent to conmon) because kubelet undoes the log rotation on error,
// and the goroutine count must settle back to baseline, proving repeated
// failing calls accumulate no stranded watcher goroutines.
func TestRuntimeOCIReopenContainerLogWatchFailureIsBestEffort(t *testing.T) {
	ctx := context.Background()
	r := &runtimeOCI{}

	bundleDir := t.TempDir()
	writeCtl(t, bundleDir)

	// Log directory intentionally absent so watcher.Add fails.
	missingDir := filepath.Join(t.TempDir(), "missing-log-dir")
	logPath := filepath.Join(missingDir, "ctr.log")
	c := newReopenTestContainer(t, bundleDir, logPath)

	if err := r.ReopenContainerLog(ctx, c); err != nil {
		t.Fatalf("expected best-effort nil error for missing log directory, got %v", err)
	}

	ctl, err := os.ReadFile(filepath.Join(bundleDir, "ctl"))
	if err != nil {
		t.Fatalf("read ctl file: %v", err)
	}

	if got, want := string(ctl), "2 0 0\n"; got != want {
		t.Fatalf("ctl file = %q, want reopen command %q", got, want)
	}

	// Repeated requests must keep succeeding the same way instead of
	// accumulating stranded watcher goroutines.
	baseline := runtime.NumGoroutine()

	for range 20 {
		if err := r.ReopenContainerLog(ctx, c); err != nil {
			t.Fatalf("expected nil error on repeated call, got %v", err)
		}
	}

	// The failing path launches no goroutine, so the count must settle back
	// to baseline instead of growing with each request.
	deadline := time.Now().Add(10 * time.Second)

	for {
		if n := runtime.NumGoroutine(); n <= baseline {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("goroutine count did not settle: baseline %d, now %d", baseline, runtime.NumGoroutine())
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// TestRuntimeOCIReopenContainerLogSuccessPath ensures the reordered watch
// still observes the expected log file creation.
func TestRuntimeOCIReopenContainerLogSuccessPath(t *testing.T) {
	ctx := context.Background()
	r := &runtimeOCI{}

	bundleDir := t.TempDir()
	writeCtl(t, bundleDir)

	logDir := t.TempDir()
	logPath := filepath.Join(logDir, "ctr.log")
	c := newReopenTestContainer(t, bundleDir, logPath)

	// Recreate the log file in a poll loop so the create event is guaranteed
	// to fire after the watch is registered, no matter when Add happens.
	stop := make(chan struct{})
	defer close(stop)

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = os.Remove(logPath)

				if err := os.WriteFile(logPath, []byte("log"), 0o644); err != nil {
					t.Errorf("failed to rewrite log file: %v", err)

					return
				}
			}
		}
	}()

	done := make(chan error, 1)

	go func() {
		done <- r.ReopenContainerLog(ctx, c)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ReopenContainerLog returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for ReopenContainerLog success path")
	}
}
