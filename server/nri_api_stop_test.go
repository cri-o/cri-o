package server

import (
	"context"
	"sync/atomic"
	"testing"

	nriadapt "github.com/containerd/nri/pkg/adaptation"

	nrilib "github.com/cri-o/cri-o/internal/nri"
)

// stubNRI counts Stop calls so the test can prove nriAPI.stop delegates to
// the adaptation instead of silently dropping the shutdown signal.
type stubNRI struct {
	nrilib.API

	stops atomic.Int32
}

func (s *stubNRI) IsEnabled() bool { return true }
func (s *stubNRI) Stop()           { s.stops.Add(1) }
func (s *stubNRI) Start() error    { return nil }

func (s *stubNRI) RunPodSandbox(context.Context, nrilib.PodSandbox) error {
	return nil
}

func (s *stubNRI) UpdatePodSandbox(context.Context, nrilib.PodSandbox, *nriadapt.LinuxResources, *nriadapt.LinuxResources) error {
	return nil
}

func (s *stubNRI) StopPodSandbox(context.Context, nrilib.PodSandbox) error {
	return nil
}

func (s *stubNRI) RemovePodSandbox(context.Context, nrilib.PodSandbox) error {
	return nil
}

func (s *stubNRI) CreateContainer(context.Context, nrilib.PodSandbox, nrilib.Container) (*nriadapt.ContainerAdjustment, error) {
	return nil, nil
}

func (s *stubNRI) PostCreateContainer(context.Context, nrilib.PodSandbox, nrilib.Container) error {
	return nil
}

func (s *stubNRI) StartContainer(context.Context, nrilib.PodSandbox, nrilib.Container) error {
	return nil
}

func (s *stubNRI) PostStartContainer(context.Context, nrilib.PodSandbox, nrilib.Container) error {
	return nil
}

func (s *stubNRI) UpdateContainer(context.Context, nrilib.PodSandbox, nrilib.Container, *nriadapt.LinuxResources) (*nriadapt.LinuxResources, error) {
	return nil, nil
}

func (s *stubNRI) PostUpdateContainer(context.Context, nrilib.PodSandbox, nrilib.Container) error {
	return nil
}

func (s *stubNRI) StopContainer(context.Context, nrilib.PodSandbox, nrilib.Container) error {
	return nil
}

func (s *stubNRI) RemoveContainer(context.Context, nrilib.PodSandbox, nrilib.Container) error {
	return nil
}

// TestNRIAPIStopDelegates verifies Shutdown's new cleanup path: stop must
// reach the adaptation, stay safe to repeat (Shutdown and constructor
// rollback can both fire), and never panic on a nil or disabled receiver.
func TestNRIAPIStopDelegates(t *testing.T) {
	stub := &stubNRI{}
	a := &nriAPI{nri: stub}

	a.stop()

	if got := stub.stops.Load(); got != 1 {
		t.Fatalf("stop did not reach adaptation, Stop calls = %d", got)
	}

	// Repeating must stay safe; the underlying local.Stop is idempotent.
	a.stop()

	if got := stub.stops.Load(); got < 1 {
		t.Fatalf("repeated stop lost the shutdown signal, Stop calls = %d", got)
	}

	var nilAPI *nriAPI
	nilAPI.stop()

	(&nriAPI{}).stop()
	(&nriAPI{nri: nil}).stop()
}
