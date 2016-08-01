package server

import (
	"fmt"
	"os"

	"github.com/mrunalp/ocid/oci"
	"github.com/mrunalp/ocid/utils"
)

const (
	runtimeAPIVersion = "v1alpha1"
	imageStore        = "/var/lib/ocid/images"
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

	if err := os.MkdirAll(imageStore, 0755); err != nil {
		return nil, err
	}

	r, err := oci.New(runtimePath, containerDir)
	if err != nil {
		return nil, err
	}
	sandboxes := make(map[string]*sandbox)
	containers := make(map[string]*oci.Container)
	return &Server{
		runtime:    r,
		sandboxDir: sandboxDir,
		state: &serverState{
			sandboxes:  sandboxes,
			containers: containers,
		},
	}, nil
}

type serverState struct {
	sandboxes  map[string]*sandbox
	containers map[string]*oci.Container
}

type sandbox struct {
	name       string
	logDir     string
	labels     map[string]string
	containers []*oci.Container
}

func (s *Server) addSandbox(sb *sandbox) {
	s.state.sandboxes[sb.name] = sb
}

func (s *Server) hasSandbox(name string) bool {
	_, ok := s.state.sandboxes[name]
	return ok
}

func (s *sandbox) addContainer(c *oci.Container) {
	s.containers = append(s.containers, c)
}

func (s *Server) addContainer(c *oci.Container) {
	sandbox := s.state.sandboxes[c.Sandbox()]
	sandbox.addContainer(c)
	s.state.containers[c.Name()] = c
}
