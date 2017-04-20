package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/containers/image/types"
	sstorage "github.com/containers/storage/storage"
	"github.com/docker/docker/pkg/stringid"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/pkg/ocicni"
	"github.com/kubernetes-incubator/cri-o/pkg/storage"
	"github.com/kubernetes-incubator/cri-o/server/apparmor"
	"github.com/kubernetes-incubator/cri-o/server/sandbox"
	"github.com/kubernetes-incubator/cri-o/server/seccomp"
	"github.com/kubernetes-incubator/cri-o/server/state"
)

const (
	runtimeAPIVersion = "v1alpha1"
)

// Server implements the RuntimeService and ImageService
type Server struct {
	config       Config
	runtime      *oci.Runtime
	store        sstorage.Store
	images       storage.ImageServer
	storage      storage.RuntimeServer
	state        state.Store
	netPlugin    ocicni.CNIPlugin
	imageContext *types.SystemContext

	seccompEnabled bool
	seccompProfile seccomp.Seccomp

	appArmorEnabled bool
	appArmorProfile string
}

// Shutdown attempts to shut down the server's storage cleanly
func (s *Server) Shutdown() error {
	_, err := s.store.Shutdown(false)
	return err
}

// New creates a new Server with options provided
func New(config *Config) (*Server, error) {
	store, err := sstorage.GetStore(sstorage.StoreOptions{
		RunRoot:            config.RunRoot,
		GraphRoot:          config.Root,
		GraphDriverName:    config.Storage,
		GraphDriverOptions: config.StorageOptions,
	})
	if err != nil {
		return nil, err
	}

	imageService, err := storage.GetImageService(store, config.DefaultTransport)
	if err != nil {
		return nil, err
	}

	storageRuntimeService := storage.GetRuntimeService(imageService, config.PauseImage)
	if err != nil {
		return nil, err
	}

	r, err := oci.New(config.Runtime, config.RuntimeHostPrivileged, config.Conmon, config.ConmonEnv, config.CgroupManager)
	if err != nil {
		return nil, err
	}
	netPlugin, err := ocicni.InitCNI(config.NetworkDir, config.PluginDir)
	if err != nil {
		return nil, err
	}
	// TODO: Should we put this elsewhere? Separate directory specified in the config?
	state, err := state.NewFileState(filepath.Join(config.RunRoot, "ocid_state"), r)
	if err != nil {
		return nil, err
	}
	s := &Server{
		runtime:         r,
		store:           store,
		images:          imageService,
		storage:         storageRuntimeService,
		netPlugin:       netPlugin,
		config:          *config,
		state:           state,
		seccompEnabled:  seccomp.IsEnabled(),
		appArmorEnabled: apparmor.IsEnabled(),
		appArmorProfile: config.ApparmorProfile,
	}
	if s.seccompEnabled {
		seccompProfile, err := ioutil.ReadFile(config.SeccompProfile)
		if err != nil {
			return nil, fmt.Errorf("opening seccomp profile (%s) failed: %v", config.SeccompProfile, err)
		}
		var seccompConfig seccomp.Seccomp
		if err := json.Unmarshal(seccompProfile, &seccompConfig); err != nil {
			return nil, fmt.Errorf("decoding seccomp profile failed: %v", err)
		}
		s.seccompProfile = seccompConfig
	}

	if s.appArmorEnabled && s.appArmorProfile == apparmor.DefaultApparmorProfile {
		if err := apparmor.EnsureDefaultApparmorProfile(); err != nil {
			return nil, fmt.Errorf("ensuring the default apparmor profile is installed failed: %v", err)
		}
	}

	s.imageContext = &types.SystemContext{
		SignaturePolicyPath: config.ImageConfig.SignaturePolicyPath,
	}

	return s, nil
}

func (s *Server) addSandbox(sb *sandbox.Sandbox) error {
	return s.state.AddSandbox(sb)
}

func (s *Server) getSandbox(id string) (*sandbox.Sandbox, error) {
	return s.state.GetSandbox(id)
}

func (s *Server) hasSandbox(id string) bool {
	return s.state.HasSandbox(id)
}

func (s *Server) removeSandbox(id string) error {
	return s.state.DeleteSandbox(id)
}

func (s *Server) addContainer(c *oci.Container) error {
	return s.state.AddContainer(c)
}

func (s *Server) getContainer(id string) (*oci.Container, error) {
	sbID, err := s.state.GetContainerSandbox(id)
	if err != nil {
		return nil, err
	}

	return s.state.GetContainer(id, sbID)
}

func (s *Server) removeContainer(c *oci.Container) error {
	return s.state.DeleteContainer(c.ID(), c.Sandbox())
}

func (s *Server) generatePodIDandName(name string, namespace string, attempt uint32) (string, string, error) {
	var (
		err error
		id  = stringid.GenerateNonCryptoID()
	)
	if namespace == "" {
		namespace = sandbox.PodDefaultNamespace
	}

	return id, fmt.Sprintf("%s-%s-%v", namespace, name, attempt), err
}

func (s *Server) getPodSandboxFromRequest(podSandboxID string) (*sandbox.Sandbox, error) {
	if podSandboxID == "" {
		return nil, sandbox.ErrSandboxIDEmpty
	}

	sb, err := s.state.LookupSandboxByID(podSandboxID)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve pod sandbox with ID starting with %v: %v", podSandboxID, err)
	}

	return sb, nil
}
