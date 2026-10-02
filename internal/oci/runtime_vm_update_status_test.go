package oci_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

func newTestContainerWithBundle(t *testing.T, bundlePath string) *oci.Container {
	t.Helper()

	c, err := oci.NewContainer("test-ctr-id", "test-ctr", bundlePath, "",
		map[string]string{}, map[string]string{},
		map[string]string{}, "", nil, nil, "",
		&types.ContainerMetadata{}, "test-sandbox", false,
		false, false, "", "", time.Now(), "")
	if err != nil {
		t.Fatalf("NewContainer: %v", err)
	}

	return c
}

func TestUpdateContainerStatus_ShimAddressMissing_MarksStopped(t *testing.T) {
	bundleDir := t.TempDir()
	// Do NOT create an "address" file — simulates shim exited and cleaned up.

	c := newTestContainerWithBundle(t, bundleDir)
	state := &oci.ContainerState{}
	state.Status = oci.ContainerStateCreated
	c.SetState(state)

	r := oci.NewRuntimeVM(&libconfig.RuntimeHandler{})

	err := r.UpdateContainerStatus(context.Background(), c)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if c.StateNoLock().Status != oci.ContainerStateStopped {
		t.Errorf("expected status %q, got %q", oci.ContainerStateStopped, c.StateNoLock().Status)
	}

	if c.StateNoLock().Finished.IsZero() {
		t.Error("expected Finished timestamp to be set")
	}
}

func TestUpdateContainerStatus_ShimAddressMissing_AlreadyStopped(t *testing.T) {
	bundleDir := t.TempDir()

	c := newTestContainerWithBundle(t, bundleDir)
	state := &oci.ContainerState{}
	state.Status = oci.ContainerStateStopped
	c.SetState(state)

	r := oci.NewRuntimeVM(&libconfig.RuntimeHandler{})

	err := r.UpdateContainerStatus(context.Background(), c)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if c.StateNoLock().Status != oci.ContainerStateStopped {
		t.Errorf("expected status %q, got %q", oci.ContainerStateStopped, c.StateNoLock().Status)
	}
}

func TestUpdateContainerStatus_ShimConnectionFails_MarksStopped(t *testing.T) {
	bundleDir := t.TempDir()

	addressPath := filepath.Join(bundleDir, "address")

	err := os.WriteFile(addressPath, []byte("unix:///nonexistent/shim.sock\n"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := newTestContainerWithBundle(t, bundleDir)
	state := &oci.ContainerState{}
	state.Status = oci.ContainerStateRunning
	c.SetState(state)

	r := oci.NewRuntimeVM(&libconfig.RuntimeHandler{})

	err = r.UpdateContainerStatus(context.Background(), c)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if c.StateNoLock().Status != oci.ContainerStateStopped {
		t.Errorf("expected status %q, got %q", oci.ContainerStateStopped, c.StateNoLock().Status)
	}

	if c.StateNoLock().Finished.IsZero() {
		t.Error("expected Finished timestamp to be set")
	}
}

func TestUpdateContainerStatus_ShimConnectionFails_AlreadyStopped_PreservesFinished(t *testing.T) {
	bundleDir := t.TempDir()

	addressPath := filepath.Join(bundleDir, "address")

	err := os.WriteFile(addressPath, []byte("unix:///nonexistent/shim.sock\n"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	originalFinished := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	c := newTestContainerWithBundle(t, bundleDir)
	state := &oci.ContainerState{}
	state.Status = oci.ContainerStateStopped
	state.Finished = originalFinished
	c.SetState(state)

	r := oci.NewRuntimeVM(&libconfig.RuntimeHandler{})

	err = r.UpdateContainerStatus(context.Background(), c)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if c.StateNoLock().Status != oci.ContainerStateStopped {
		t.Errorf("expected status %q, got %q",
			oci.ContainerStateStopped, c.StateNoLock().Status)
	}

	if !c.StateNoLock().Finished.Equal(originalFinished) {
		t.Errorf("expected Finished preserved as %v, got %v",
			originalFinished, c.StateNoLock().Finished)
	}
}

func TestUpdateContainerStatus_ShimAddressUnreadable_ReturnsError(t *testing.T) {
	bundleDir := t.TempDir()

	// Create "address" as a directory so ReadFile fails with a
	// non-ErrNotExist error (works even as root).
	addressPath := filepath.Join(bundleDir, "address")
	if err := os.Mkdir(addressPath, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	c := newTestContainerWithBundle(t, bundleDir)
	state := &oci.ContainerState{}
	state.Status = oci.ContainerStateRunning
	c.SetState(state)

	r := oci.NewRuntimeVM(&libconfig.RuntimeHandler{})

	err := r.UpdateContainerStatus(context.Background(), c)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if c.StateNoLock().Status != oci.ContainerStateRunning {
		t.Errorf("expected status unchanged %q, got %q",
			oci.ContainerStateRunning, c.StateNoLock().Status)
	}
}
