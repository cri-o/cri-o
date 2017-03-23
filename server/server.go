package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/types"
	sstorage "github.com/containers/storage/storage"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/pkg/ocicni"
	"github.com/kubernetes-incubator/cri-o/pkg/storage"
	"github.com/kubernetes-incubator/cri-o/server/apparmor"
	"github.com/kubernetes-incubator/cri-o/server/seccomp"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux/label"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
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
	updateLock   sync.RWMutex
	state        StateStore
	netPlugin    ocicni.CNIPlugin
	imageContext *types.SystemContext

	seccompEnabled bool
	seccompProfile seccomp.Seccomp

	appArmorEnabled bool
	appArmorProfile string
}

func (s *Server) loadContainer(id string) error {
	config, err := s.store.GetFromContainerDirectory(id, "config.json")
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

	var metadata pb.ContainerMetadata
	if err = json.Unmarshal([]byte(m.Annotations["ocid/metadata"]), &metadata); err != nil {
		return err
	}
	sb, err := s.getSandbox(m.Annotations["ocid/sandbox_id"])
	if err != nil {
		return fmt.Errorf("could not get sandbox with id %s, skipping", m.Annotations["ocid/sandbox_id"])
	}

	var tty bool
	if v := m.Annotations["ocid/tty"]; v == "true" {
		tty = true
	}
	containerPath, err := s.store.GetContainerRunDirectory(id)
	if err != nil {
		return err
	}

	var img *pb.ImageSpec
	image, ok := m.Annotations["ocid/image"]
	if ok {
		img = &pb.ImageSpec{
			Image: image,
		}
	}

	annotations := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations["ocid/annotations"]), &annotations); err != nil {
		return err
	}

	ctr, err := oci.NewContainer(id, name, containerPath, m.Annotations["ocid/log_path"], sb.netNs(), labels, annotations, img, &metadata, sb.id, tty, sb.privileged)
	if err != nil {
		return err
	}
	if err = s.runtime.UpdateStatus(ctr); err != nil {
		return fmt.Errorf("error updating status for container %s: %v", ctr.ID(), err)
	}
	return s.addContainer(ctr)
}

func configNetNsPath(spec rspec.Spec) (string, error) {
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type != rspec.NetworkNamespace {
			continue
		}

		if ns.Path == "" {
			return "", fmt.Errorf("empty networking namespace")
		}

		return ns.Path, nil
	}

	return "", fmt.Errorf("missing networking namespace")
}

func (s *Server) loadSandbox(id string) error {
	config, err := s.store.GetFromContainerDirectory(id, "config.json")
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
	var metadata pb.PodSandboxMetadata
	if err = json.Unmarshal([]byte(m.Annotations["ocid/metadata"]), &metadata); err != nil {
		return err
	}

	processLabel, mountLabel, err := label.InitLabels(label.DupSecOpt(m.Process.SelinuxLabel))
	if err != nil {
		return err
	}

	annotations := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations["ocid/annotations"]), &annotations); err != nil {
		return err
	}

	privileged := m.Annotations["ocid/privileged_runtime"] == "true"

	sb := &sandbox{
		id:           id,
		name:         name,
		logDir:       filepath.Dir(m.Annotations["ocid/log_path"]),
		labels:       labels,
		containers:   oci.NewMemoryStore(),
		processLabel: processLabel,
		mountLabel:   mountLabel,
		annotations:  annotations,
		metadata:     &metadata,
		shmPath:      m.Annotations["ocid/shm_path"],
		privileged:   privileged,
		resolvPath:   m.Annotations["ocid/resolv_path"],
	}

	// We add a netNS only if we can load a permanent one.
	// Otherwise, the sandbox will live in the host namespace.
	netNsPath, err := configNetNsPath(m)
	if err == nil {
		netNS, nsErr := netNsGet(netNsPath, sb.name)
		// If we can't load the networking namespace
		// because it's closed, we just set the sb netns
		// pointer to nil. Otherwise we return an error.
		if nsErr != nil && nsErr != errSandboxClosedNetNS {
			return nsErr
		}

		sb.netns = netNS
	}

	sandboxPath, err := s.store.GetContainerRunDirectory(id)
	if err != nil {
		return err
	}

	scontainer, err := oci.NewContainer(m.Annotations["ocid/container_id"], m.Annotations["ocid/container_name"], sandboxPath, m.Annotations["ocid/log_path"], sb.netNs(), labels, annotations, nil, nil, id, false, privileged)
	if err != nil {
		return err
	}
	if err = s.runtime.UpdateStatus(scontainer); err != nil {
		return fmt.Errorf("error updating status for pod sandbox infra container %s: %v", scontainer.ID(), err)
	}
	if err = label.ReserveLabel(processLabel); err != nil {
		return err
	}
	sb.infraContainer = scontainer

	return s.addSandbox(sb)
}

func (s *Server) restore() {
	containers, err := s.store.Containers()
	if err != nil && !os.IsNotExist(err) {
		logrus.Warnf("could not read containers and sandboxes: %v", err)
	}
	pods := map[string]*storage.RuntimeContainerMetadata{}
	podContainers := map[string]*storage.RuntimeContainerMetadata{}
	for _, container := range containers {
		metadata, err2 := s.storage.GetContainerMetadata(container.ID)
		if err2 != nil {
			logrus.Warnf("error parsing metadata for %s: %v, ignoring", container.ID, err2)
			continue
		}
		if metadata.Pod {
			pods[container.ID] = &metadata
		} else {
			podContainers[container.ID] = &metadata
		}
	}
	for containerID, metadata := range pods {
		if err = s.loadSandbox(containerID); err != nil {
			logrus.Warnf("could not restore sandbox %s container %s: %v", metadata.PodID, containerID, err)
		}
	}
	for containerID := range podContainers {
		if err := s.loadContainer(containerID); err != nil {
			logrus.Warnf("could not restore container %s: %v", containerID, err)
		}
	}
}

// Update makes changes to the server's state (lists of pods and containers) to
// reflect the list of pods and containers that are stored on disk, possibly
// having been modified by other parties
func (s *Server) Update() {
	logrus.Debugf("updating sandbox and container information")
	if err := s.update(); err != nil {
		logrus.Errorf("error updating sandbox and container information: %v", err)
	}
}

func (s *Server) update() error {
	s.updateLock.Lock()
	defer s.updateLock.Unlock()

	containers, err := s.store.Containers()
	if err != nil && !os.IsNotExist(err) {
		logrus.Warnf("could not read containers and sandboxes: %v", err)
		return err
	}
	newPods := map[string]*storage.RuntimeContainerMetadata{}
	oldPods := map[string]string{}
	removedPods := map[string]string{}
	newPodContainers := map[string]*storage.RuntimeContainerMetadata{}
	oldPodContainers := map[string]string{}
	removedPodContainers := map[string]string{}
	for _, container := range containers {
		if s.hasSandbox(container.ID) {
			// FIXME: do we need to reload/update any info about the sandbox?
			oldPods[container.ID] = container.ID
			oldPodContainers[container.ID] = container.ID
			continue
		}
		if _, err := s.getContainer(container.ID); err == nil {
			// FIXME: do we need to reload/update any info about the container?
			oldPodContainers[container.ID] = container.ID
			continue
		}
		// not previously known, so figure out what it is
		metadata, err2 := s.storage.GetContainerMetadata(container.ID)
		if err2 != nil {
			logrus.Errorf("error parsing metadata for %s: %v, ignoring", container.ID, err2)
			continue
		}
		if metadata.Pod {
			newPods[container.ID] = &metadata
		} else {
			newPodContainers[container.ID] = &metadata
		}
	}

	// TODO this will not check pod infra containers - should we care about this?
	stateContainers, err := s.state.GetAllContainers()
	if err != nil {
		return fmt.Errorf("error retrieving containers list: %v", err)
	}
	for _, ctr := range stateContainers {
		if _, ok := oldPodContainers[ctr.ID()]; !ok {
			// this container's ID wasn't in the updated list -> removed
			removedPodContainers[ctr.ID()] = ctr.ID()
		}
	}

	for removedPodContainer := range removedPodContainers {
		// forget this container
		c, err := s.getContainer(removedPodContainer)
		if err != nil {
			logrus.Warnf("bad state when getting container removed %+v", removedPodContainer)
			continue
		}
		if err := s.removeContainer(c); err != nil {
			return fmt.Errorf("error forgetting removed pod container %s: %v", c.ID(), err)
		}
		logrus.Debugf("forgetting removed pod container %s", c.ID())
	}

	pods, err := s.state.GetAllSandboxes()
	if err != nil {
		return fmt.Errorf("error retrieving pods list: %v", err)
	}
	for _, pod := range pods {
		if _, ok := oldPods[pod.id]; !ok {
			// this pod's ID wasn't in the updated list -> removed
			removedPods[pod.id] = pod.id
		}
	}

	for removedPod := range removedPods {
		// forget this pod
		sb, err := s.getSandbox(removedPod)
		if err != nil {
			logrus.Warnf("bad state when getting pod to remove %+v", removedPod)
			continue
		}
		if err := s.removeSandbox(sb.id); err != nil {
			return fmt.Errorf("error removing sandbox %s: %v", sb.id, err)
		}
		sb.infraContainer = nil
		logrus.Debugf("forgetting removed pod %s", sb.id)
	}
	for sandboxID := range newPods {
		// load this pod
		if err = s.loadSandbox(sandboxID); err != nil {
			logrus.Warnf("could not load new pod sandbox %s: %v, ignoring", sandboxID, err)
		} else {
			logrus.Debugf("loaded new pod sandbox %s", sandboxID, err)
		}
	}
	for containerID := range newPodContainers {
		// load this container
		if err = s.loadContainer(containerID); err != nil {
			logrus.Warnf("could not load new sandbox container %s: %v, ignoring", containerID, err)
		} else {
			logrus.Debugf("loaded new pod container %s", containerID, err)
		}
	}
	return nil
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
	s := &Server{
		runtime:         r,
		store:           store,
		images:          imageService,
		storage:         storageRuntimeService,
		netPlugin:       netPlugin,
		config:          *config,
		state:           NewInMemoryState(),
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

	s.restore()

	return s, nil
}

func (s *Server) addSandbox(sb *sandbox) error {
	return s.state.AddSandbox(sb)
}

func (s *Server) getSandbox(id string) (*sandbox, error) {
	return s.state.GetSandbox(id)
}

func (s *Server) hasSandbox(id string) bool {
	return s.state.HasSandbox(id)
}

func (s *Server) removeSandbox(id string) error {
	return s.state.DeleteSandbox(id)
}

func (s *Server) addContainer(c *oci.Container) error {
	return s.state.AddContainer(c, c.Sandbox())
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
