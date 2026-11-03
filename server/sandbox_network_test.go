package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/pkg/config"
	ocicnitypesmock "github.com/cri-o/cri-o/test/mocks/ocicni"
)

func newServerWithNotReadyCNI(t *testing.T) *Server {
	t.Helper()

	ctrl := gomock.NewController(t)
	pluginMock := ocicnitypesmock.NewMockCNIPlugin(ctrl)
	pluginMock.EXPECT().Status().Return(errors.New("not ready")).AnyTimes()
	pluginMock.EXPECT().StatusWithContext(gomock.Any()).Return(errors.New("not ready")).AnyTimes()
	pluginMock.EXPECT().Shutdown().Return(nil).AnyTimes()

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	if err := cfg.SetCNIPlugin(pluginMock); err != nil {
		t.Fatalf("SetCNIPlugin: %v", err)
	}

	return &Server{
		config: *cfg,
	}
}

func TestWaitForCNIPluginReturnsOnContextCancel(t *testing.T) {
	t.Parallel()

	s := newServerWithNotReadyCNI(t)
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

func TestWaitForCNIPluginReturnsOnCNIManagerShutdown(t *testing.T) {
	t.Parallel()

	s := newServerWithNotReadyCNI(t)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.waitForCNIPlugin(context.Background(), "test-sandbox")
	}()

	select {
	case err := <-errCh:
		t.Fatalf("waitForCNIPlugin returned before CNI manager shutdown: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	s.config.CNIManagerShutdown()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from waitForCNIPlugin after shutdown, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForCNIPlugin did not return after CNI manager shutdown")
	}
}
