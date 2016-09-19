package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/ocid/oci"
	"github.com/kubernetes-incubator/ocid/utils"
	"github.com/rajatchopra/ocicni"
)

const (
	runtimeAPIVersion = "v1alpha1"
	imageStore        = "/var/lib/ocid/images"
)

// Server implements the RuntimeService and ImageService
type Server struct {
	runtime    *oci.Runtime
	sandboxDir string
	stateLock  sync.Mutex
	state      *serverState
	netPlugin  ocicni.CNIPlugin
}

func (s *Server) loadSandboxes() error {
	err := filepath.Walk(s.sandboxDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if path == s.sandboxDir {
			return nil
		}
		metaJSON, err := ioutil.ReadFile(filepath.Join(path, "metadata.json"))
		if err != nil {
			return err
		}
		var m metadata
		if err2 := json.Unmarshal(metaJSON, &m); err2 != nil {
			return err2
		}
		sname, err := filepath.Rel(s.sandboxDir, path)
		if err != nil {
			return err
		}
		s.addSandbox(&sandbox{
			name:       sname,
			logDir:     m.LogDir,
			labels:     m.Labels,
			containers: oci.NewMemoryStore(),
		})
		scontainer, err := oci.NewContainer(m.ContainerName, path, path, m.Labels, sname, false)
		if err != nil {
			return err
		}
		s.addContainer(scontainer)
		if err = s.runtime.UpdateStatus(scontainer); err != nil {
			logrus.Warnf("error updating status for container %s: %v", scontainer, err)
		}
		return nil
	})
	return err
}

// New creates a new Server with options provided
func New(runtimePath, sandboxDir, containerDir string) (*Server, error) {
	// TODO: This will go away later when we have wrapper process or systemd acting as
	// subreaper.
	if err := utils.SetSubreaper(1); err != nil {
		return nil, fmt.Errorf("failed to set server as subreaper: %v", err)
	}

	utils.StartReaper()

	if err := os.MkdirAll(imageStore, 0755); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return nil, err
	}

	r, err := oci.New(runtimePath, containerDir)
	if err != nil {
		return nil, err
	}
	sandboxes := make(map[string]*sandbox)
	containers := oci.NewMemoryStore()
	netPlugin, err := ocicni.InitCNI("")
	if err != nil {
		return nil, err
	}
	s := &Server{
		runtime:    r,
		netPlugin:  netPlugin,
		sandboxDir: sandboxDir,
		state: &serverState{
			sandboxes:  sandboxes,
			containers: containers,
		},
	}
	if err := s.loadSandboxes(); err != nil {
		logrus.Warnf("couldn't get sandboxes: %v", err)
	}
	logrus.Debugf("sandboxes: %v", s.state.sandboxes)
	logrus.Debugf("containers: %v", s.state.containers)
	return s, nil
}

type serverState struct {
	sandboxes  map[string]*sandbox
	containers oci.Store
}

func (s *Server) addSandbox(sb *sandbox) {
	s.stateLock.Lock()
	s.state.sandboxes[sb.name] = sb
	s.stateLock.Unlock()
}

func (s *Server) getSandbox(name string) *sandbox {
	s.stateLock.Lock()
	sb := s.state.sandboxes[name]
	s.stateLock.Unlock()
	return sb
}

func (s *Server) hasSandbox(name string) bool {
	s.stateLock.Lock()
	_, ok := s.state.sandboxes[name]
	s.stateLock.Unlock()
	return ok
}

func (s *Server) addContainer(c *oci.Container) {
	s.stateLock.Lock()
	sandbox := s.state.sandboxes[c.Sandbox()]
	sandbox.addContainer(c)
	s.state.containers.Add(c.Name(), c)
	s.stateLock.Unlock()
}

func (s *Server) getContainer(name string) *oci.Container {
	s.stateLock.Lock()
	c := s.state.containers.Get(name)
	s.stateLock.Unlock()
	return c
}

func (s *Server) removeContainer(c *oci.Container) {
	s.stateLock.Lock()
	sandbox := s.state.sandboxes[c.Sandbox()]
	sandbox.removeContainer(c)
	s.state.containers.Delete(c.Name())
	s.stateLock.Unlock()
}
