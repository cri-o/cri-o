package server

import (
	"github.com/mrunalp/ocid/oci"
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
