package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cri-o/ocicni/pkg/ocicni"

	"github.com/cri-o/cri-o/pkg/config"
)

type notReadyCNIPlugin struct{}

func (notReadyCNIPlugin) Name() string                  { return "fake" }
func (notReadyCNIPlugin) GetDefaultNetworkName() string { return "fake-net" }
func (notReadyCNIPlugin) Shutdown() error               { return nil }
func (notReadyCNIPlugin) Status() error                 { return errors.New("not ready") }

func (notReadyCNIPlugin) StatusWithContext(context.Context) error {
	return errors.New("not ready")
}

func (notReadyCNIPlugin) SetUpPod(ocicni.PodNetwork) ([]ocicni.NetResult, error) {
	return nil, nil
}

func (notReadyCNIPlugin) SetUpPodWithContext(
	context.Context, ocicni.PodNetwork,
) ([]ocicni.NetResult, error) {
	return nil, nil
}

func (notReadyCNIPlugin) TearDownPod(ocicni.PodNetwork) error { return nil }

func (notReadyCNIPlugin) TearDownPodWithContext(
	context.Context, ocicni.PodNetwork,
) error {
	return nil
}

func (notReadyCNIPlugin) GetPodNetworkStatus(
	ocicni.PodNetwork,
) ([]ocicni.NetResult, error) {
	return nil, nil
}

func (notReadyCNIPlugin) GetPodNetworkStatusWithContext(
	context.Context, ocicni.PodNetwork,
) ([]ocicni.NetResult, error) {
	return nil, nil
}

func (notReadyCNIPlugin) GC(context.Context, []*ocicni.PodNetwork) error {
	return nil
}

func TestWaitForCNIPluginReturnsOnContextCancel(t *testing.T) {
	t.Parallel()

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	if err := cfg.SetCNIPlugin(notReadyCNIPlugin{}); err != nil {
		t.Fatalf("SetCNIPlugin: %v", err)
	}

	s := &Server{
		config: *cfg,
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.waitForCNIPlugin(ctx, "test-sandbox")
	}()

	select {
	case err := <-errCh:
		t.Fatalf("waitForCNIPlugin returned before context was cancelled: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from waitForCNIPlugin after cancel, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForCNIPlugin did not return after context was cancelled")
	}
}
