package server

import (
	"github.com/mrunalp/ocid/oci"
)

const (
	runtimeAPIVersion = "v1alpha1"
)

// Server implements the RuntimeService and ImageService
type Server struct {
	runtime   *oci.Runtime
	sandboxes []*sandbox
}

// New creates a new Server with options provided
func New(runtimePath, sandboxDir, containerDir string) (*Server, error) {
	r, err := oci.New(runtimePath, sandboxDir, containerDir)
	if err != nil {
		return nil, err
	}
	return &Server{
		runtime: r,
	}, nil
}

type sandbox struct {
	name   string
	logDir string
	labels map[string]string
}
