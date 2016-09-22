package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/pkg/registrar"
	"github.com/docker/docker/pkg/truncindex"
	"github.com/kubernetes-incubator/ocid/oci"
	"github.com/kubernetes-incubator/ocid/utils"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/rajatchopra/ocicni"
)

const (
	runtimeAPIVersion = "v1alpha1"
	imageStore        = "/var/lib/ocid/images"
)

// Server implements the RuntimeService and ImageService
type Server struct {
	runtime      *oci.Runtime
	sandboxDir   string
	stateLock    sync.Mutex
	state        *serverState
	netPlugin    ocicni.CNIPlugin
	podNameIndex *registrar.Registrar
	podIDIndex   *truncindex.TruncIndex
}

func (s *Server) loadSandbox(id string) error {
	config, err := ioutil.ReadFile(filepath.Join(s.sandboxDir, id, "config.json"))
	if err != nil {
		return err
	}
	var m rspec.Spec
	if err = json.Unmarshal(config, &m); err != nil {
		return err
	}
	labels := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations["ocid/labels"]), &labels); err != nil {
		return err
	}
	name := m.Annotations["ocid/name"]
	name, err = s.reservePodName(id, name)
	if err != nil {
		return err
	}
	s.addSandbox(&sandbox{
		id:         id,
		name:       name,
		logDir:     m.Annotations["ocid/log_path"],
		labels:     labels,
		containers: oci.NewMemoryStore(),
	})
	sandboxPath := filepath.Join(s.sandboxDir, id)
	scontainer, err := oci.NewContainer(m.Annotations["ocid/container_name"], sandboxPath, sandboxPath, labels, id, false)
	if err != nil {
		return err
	}
	s.addContainer(scontainer)
	if err = s.runtime.UpdateStatus(scontainer); err != nil {
		logrus.Warnf("error updating status for container %s: %v", scontainer, err)
	}
	if err = s.podIDIndex.Add(id); err != nil {
		return err
	}
	return nil
}

func (s *Server) restore() error {
	dir, err := ioutil.ReadDir(s.sandboxDir)
	if err != nil {
		return err
	}
	for _, v := range dir {
		if err := s.loadSandbox(v.Name()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) reservePodName(id, name string) (string, error) {
	if err := s.podNameIndex.Reserve(name, id); err != nil {
		if err == registrar.ErrNameReserved {
			id, err := s.podNameIndex.Get(name)
			if err != nil {
				logrus.Warnf("name %s already reserved for %s", name, id)
				return "", err
			}
			return "", fmt.Errorf("conflict, name %s already reserver", name)
		}
		return "", fmt.Errorf("error reserving name %s", name)
	}
	return name, nil
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
	s.podIDIndex = truncindex.NewTruncIndex([]string{})
	s.podNameIndex = registrar.NewRegistrar()
	if err := s.restore(); err != nil {
		logrus.Warnf("couldn't restore: %v", err)
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
	s.state.sandboxes[sb.id] = sb
	s.stateLock.Unlock()
}

func (s *Server) getSandbox(id string) *sandbox {
	s.stateLock.Lock()
	sb := s.state.sandboxes[id]
	s.stateLock.Unlock()
	return sb
}

func (s *Server) hasSandbox(id string) bool {
	s.stateLock.Lock()
	_, ok := s.state.sandboxes[id]
	s.stateLock.Unlock()
	return ok
}

func (s *Server) addContainer(c *oci.Container) {
	s.stateLock.Lock()
	sandbox := s.state.sandboxes[c.Sandbox()]
	// TODO(runcom): handle !ok above!!! otherwise it panics!
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
