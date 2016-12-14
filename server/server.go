package server

import (
	"fmt"
	"os"

	runtimeManager "github.com/kubernetes-incubator/cri-o/manager"
	"github.com/kubernetes-incubator/cri-o/utils"
)

const (
	runtimeAPIVersion = "v1alpha1"
)

// Server implements the RuntimeService and ImageService
type Server struct {
	manager *runtimeManager.Manager
}

// New creates a new Server with options provided
func New(config *runtimeManager.Config) (*Server, error) {
	// TODO: This will go away later when we have wrapper process or systemd acting as
	// subreaper.
	if err := utils.SetSubreaper(1); err != nil {
		return nil, fmt.Errorf("failed to set server as subreaper: %v", err)
	}

	utils.StartReaper()

	if err := os.MkdirAll(config.ImageDir, 0755); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(config.SandboxDir, 0755); err != nil {
		return nil, err
	}

	manager, err := runtimeManager.New(config)
	if err != nil {
		return nil, err
	}
	s := &Server{
		manager: manager,
	}

	return s, nil
}
