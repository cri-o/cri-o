package server

import (
	"fmt"

	"github.com/mrunalp/ocid/oci"
	"github.com/mrunalp/ocid/utils"
)

const (
	runtimeAPIVersion = "v1alpha1"
)

// Server implements the RuntimeService and ImageService
type Server struct {
	runtime    *oci.Runtime
	sandboxDir string
	state      *serverState
}

// New creates a new Server with options provided
func New(runtimePath, sandboxDir, containerDir string) (*Server, error) {
	// TODO: This will go away later when we have wrapper process or systemd acting as
	// subreaper.
	if err := utils.SetSubreaper(1); err != nil {
		return nil, fmt.Errorf("failed to set server as subreaper: %v", err)
	}

	r, err := oci.New(runtimePath, containerDir)
	if err != nil {
		return nil, err
	}
	sandboxes := make(map[string]*sandbox)
	return &Server{
		runtime:    r,
		sandboxDir: sandboxDir,
		state: &serverState{
			sandboxes: sandboxes,
		},
	}, nil
}

type serverState struct {
	sandboxes map[string]*sandbox
}

type sandbox struct {
	name   string
	logDir string
	labels map[string]string
}

func (s *Server) addSandbox(sb *sandbox) {
	s.state.sandboxes[sb.name] = sb
}
