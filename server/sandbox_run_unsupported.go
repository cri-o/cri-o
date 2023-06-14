//go:build !linux
// +build !linux

package server

import (
	"context"
	"fmt"

	"github.com/containers/storage/pkg/idtools"
	libsandbox "github.com/cri-o/cri-o/internal/lib/sandbox"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) runPodSandbox(ctx context.Context, req *types.RunPodSandboxRequest) (*types.RunPodSandboxResponse, error) {
	return nil, fmt.Errorf("unsupported")
}

func (s *Server) getSandboxIDMappings(ctx context.Context, sb *libsandbox.Sandbox) (*idtools.IDMappings, error) {
	return nil, fmt.Errorf("unsupported")
}
