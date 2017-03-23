package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/containers/image/types"
	sstorage "github.com/containers/storage"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/pkg/annotations"
	"github.com/kubernetes-incubator/cri-o/pkg/ocicni"
	"github.com/kubernetes-incubator/cri-o/pkg/storage"
	"github.com/kubernetes-incubator/cri-o/server/apparmor"
	"github.com/kubernetes-incubator/cri-o/server/seccomp"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux/label"
	knet "k8s.io/apimachinery/pkg/util/net"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
	"k8s.io/kubernetes/pkg/kubelet/network/hostport"
	"k8s.io/kubernetes/pkg/kubelet/server/streaming"
	iptablesproxy "k8s.io/kubernetes/pkg/proxy/iptables"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	utilexec "k8s.io/kubernetes/pkg/util/exec"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
)

const (
	runtimeAPIVersion = "v1alpha1"
	shutdownFile      = "/var/lib/crio/crio.shutdown"
)

func isTrue(annotaton string) bool {
	return annotaton == "true"
}

// streamService implements streaming.Runtime.
type streamService struct {
	runtimeServer *Server // needed by Exec() endpoint
	streamServer  streaming.Server
	streaming.Runtime
}

// Server implements the RuntimeService and ImageService
type Server struct {
	config               Config
	runtime              *oci.Runtime
	store                sstorage.Store
	storageImageServer   storage.ImageServer
	storageRuntimeServer storage.RuntimeServer
	updateLock           sync.RWMutex
	state                StateStore
	netPlugin            ocicni.CNIPlugin
	hostportManager      hostport.HostPortManager
	imageContext         *types.SystemContext

	seccompEnabled bool
	seccompProfile seccomp.Seccomp

	appArmorEnabled bool
	appArmorProfile string

	stream streamService
}

// GetExec returns exec stream request
func (s *Server) GetExec(req *pb.ExecRequest) (*pb.ExecResponse, error) {
	return s.stream.streamServer.GetExec(req)
}

// GetAttach returns attach stream request
func (s *Server) GetAttach(req *pb.AttachRequest) (*pb.AttachResponse, error) {
	return s.stream.streamServer.GetAttach(req)
}

// GetPortForward returns port forward stream request
func (s *Server) GetPortForward(req *pb.PortForwardRequest) (*pb.PortForwardResponse, error) {
	return s.stream.streamServer.GetPortForward(req)
}

func (s *Server) loadContainer(id string) error {
	config, err := s.store.FromContainerDirectory(id, "config.json")
	if err != nil {
		return err
	}
	var m rspec.Spec
	if err = json.Unmarshal(config, &m); err != nil {
		return err
	}
	labels := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Labels]), &labels); err != nil {
		return err
	}
	name := m.Annotations[annotations.Name]

	var metadata pb.ContainerMetadata
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Metadata]), &metadata); err != nil {
		return err
	}
	sb, err := s.getSandbox(m.Annotations[annotations.SandboxID])
	if err != nil {
		return fmt.Errorf("could not get sandbox with id %s, skipping", m.Annotations[annotations.SandboxID])
	}

	tty := isTrue(m.Annotations[annotations.TTY])
	stdin := isTrue(m.Annotations[annotations.Stdin])
	stdinOnce := isTrue(m.Annotations[annotations.StdinOnce])

	containerPath, err := s.store.ContainerRunDirectory(id)
	if err != nil {
		return err
	}

	containerDir, err := s.store.ContainerDirectory(id)
	if err != nil {
		return err
	}

	var img *pb.ImageSpec
	image, ok := m.Annotations[annotations.Image]
	if ok {
		img = &pb.ImageSpec{
			Image: image,
		}
	}

	kubeAnnotations := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Annotations]), &kubeAnnotations); err != nil {
		return err
	}

	created, err := time.Parse(time.RFC3339Nano, m.Annotations[annotations.Created])
	if err != nil {
		return err
	}

	ctr, err := oci.NewContainer(id, name, containerPath, m.Annotations[annotations.LogPath], sb.netNs(), labels, kubeAnnotations, img, &metadata, sb.id, tty, stdin, stdinOnce, sb.privileged, sb.trusted, containerDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
	if err != nil {
		return err
	}

	s.containerStateFromDisk(ctr)

	return s.addContainer(ctr)
}

func (s *Server) containerStateFromDisk(c *oci.Container) error {
	if err := c.FromDisk(); err != nil {
		return err
	}
	// ignore errors, this is a best effort to have up-to-date info about
	// a given container before its state gets stored
	s.runtime.UpdateStatus(c)

	return nil
}

func (s *Server) containerStateToDisk(c *oci.Container) error {
	// ignore errors, this is a best effort to have up-to-date info about
	// a given container before its state gets stored
	s.runtime.UpdateStatus(c)

	jsonSource, err := ioutils.NewAtomicFileWriter(c.StatePath(), 0644)
	if err != nil {
		return err
	}
	defer jsonSource.Close()
	enc := json.NewEncoder(jsonSource)
	return enc.Encode(s.runtime.ContainerStatus(c))
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
	config, err := s.store.FromContainerDirectory(id, "config.json")
	if err != nil {
		return err
	}
	var m rspec.Spec
	if err = json.Unmarshal(config, &m); err != nil {
		return err
	}
	labels := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Labels]), &labels); err != nil {
		return err
	}
	name := m.Annotations[annotations.Name]
	var metadata pb.PodSandboxMetadata
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Metadata]), &metadata); err != nil {
		return err
	}

	processLabel, mountLabel, err := label.InitLabels(label.DupSecOpt(m.Process.SelinuxLabel))
	if err != nil {
		return err
	}

	kubeAnnotations := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Annotations]), &kubeAnnotations); err != nil {
		return err
	}

	privileged := isTrue(m.Annotations[annotations.PrivilegedRuntime])
	trusted := isTrue(m.Annotations[annotations.TrustedSandbox])

	sb := &sandbox{
		id:           id,
		name:         name,
		kubeName:     m.Annotations[annotations.KubeName],
		logDir:       filepath.Dir(m.Annotations[annotations.LogPath]),
		labels:       labels,
		containers:   oci.NewMemoryStore(),
		processLabel: processLabel,
		mountLabel:   mountLabel,
		annotations:  kubeAnnotations,
		metadata:     &metadata,
		shmPath:      m.Annotations[annotations.ShmPath],
		privileged:   privileged,
		trusted:      trusted,
		resolvPath:   m.Annotations[annotations.ResolvPath],
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

	sandboxPath, err := s.store.ContainerRunDirectory(id)
	if err != nil {
		return err
	}

	sandboxDir, err := s.store.ContainerDirectory(id)
	if err != nil {
		return err
	}

	created, err := time.Parse(time.RFC3339Nano, m.Annotations[annotations.Created])
	if err != nil {
		return err
	}

	scontainer, err := oci.NewContainer(m.Annotations[annotations.ContainerID], m.Annotations[annotations.ContainerName], sandboxPath, m.Annotations[annotations.LogPath], sb.netNs(), labels, kubeAnnotations, nil, nil, id, false, false, false, privileged, trusted, sandboxDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
	if err != nil {
		return err
	}

	s.containerStateFromDisk(scontainer)

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
		metadata, err2 := s.storageRuntimeServer.GetContainerMetadata(container.ID)
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
		metadata, err2 := s.storageRuntimeServer.GetContainerMetadata(container.ID)
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

// cleanupSandboxesOnShutdown Remove all running Sandboxes on system shutdown
func (s *Server) cleanupSandboxesOnShutdown() {
	_, err := os.Stat(shutdownFile)
	if err == nil || !os.IsNotExist(err) {
		logrus.Debugf("shutting down all sandboxes, on shutdown")
		s.StopAllPodSandboxes()
		err = os.Remove(shutdownFile)
		if err != nil {
			logrus.Warnf("Failed to remove %q", shutdownFile)
		}

	}
}

// Shutdown attempts to shut down the server's storage cleanly
func (s *Server) Shutdown() error {
	// why do this on clean shutdown! we want containers left running when ocid
	// is down for whatever reason no?!
	// notice this won't trigger just on system halt but also on normal
	// ocid.service restart!!!
	s.cleanupSandboxesOnShutdown()
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

	imageService, err := storage.GetImageService(store, config.DefaultTransport, config.InsecureRegistries)
	if err != nil {
		return nil, err
	}

	storageRuntimeService := storage.GetRuntimeService(imageService, config.PauseImage)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll("/var/run/crio", 0755); err != nil {
		return nil, err
	}

	r, err := oci.New(config.Runtime, config.RuntimeUntrustedWorkload, config.DefaultWorkloadTrust, config.Conmon, config.ConmonEnv, config.CgroupManager)
	if err != nil {
		return nil, err
	}
	netPlugin, err := ocicni.InitCNI(config.NetworkDir, config.PluginDir)
	if err != nil {
		return nil, err
	}
	iptInterface := utiliptables.New(utilexec.New(), utildbus.New(), utiliptables.ProtocolIpv4)
	iptInterface.EnsureChain(utiliptables.TableNAT, iptablesproxy.KubeMarkMasqChain)
	hostportManager := hostport.NewHostportManager()
	s := &Server{
		runtime:              r,
		store:                store,
		storageImageServer:   imageService,
		storageRuntimeServer: storageRuntimeService,
		netPlugin:            netPlugin,
		hostportManager:      hostportManager,
		config:               *config,
		state:                NewInMemoryState(),
		seccompEnabled:       seccomp.IsEnabled(),
		appArmorEnabled:      apparmor.IsEnabled(),
		appArmorProfile:      config.ApparmorProfile,
	}
	if s.seccompEnabled {
		seccompProfile, fileErr := ioutil.ReadFile(config.SeccompProfile)
		if fileErr != nil {
			return nil, fmt.Errorf("opening seccomp profile (%s) failed: %v", config.SeccompProfile, fileErr)
		}
		var seccompConfig seccomp.Seccomp
		if jsonErr := json.Unmarshal(seccompProfile, &seccompConfig); jsonErr != nil {
			return nil, fmt.Errorf("decoding seccomp profile failed: %v", jsonErr)
		}
		s.seccompProfile = seccompConfig
	}

	if s.appArmorEnabled && s.appArmorProfile == apparmor.DefaultApparmorProfile {
		if apparmorErr := apparmor.EnsureDefaultApparmorProfile(); apparmorErr != nil {
			return nil, fmt.Errorf("ensuring the default apparmor profile is installed failed: %v", apparmorErr)
		}
	}

	s.imageContext = &types.SystemContext{
		SignaturePolicyPath: config.ImageConfig.SignaturePolicyPath,
	}

	s.restore()
	s.cleanupSandboxesOnShutdown()

	bindAddress := net.ParseIP(config.StreamAddress)
	if bindAddress == nil {
		bindAddress, err = knet.ChooseBindAddress(net.IP{0, 0, 0, 0})
		if err != nil {
			return nil, err
		}
	}

	_, err = net.LookupPort("tcp", config.StreamPort)
	if err != nil {
		return nil, err
	}

	// Prepare streaming server
	streamServerConfig := streaming.DefaultConfig
	streamServerConfig.Addr = net.JoinHostPort(bindAddress.String(), config.StreamPort)
	s.stream.runtimeServer = s
	s.stream.streamServer, err = streaming.NewServer(streamServerConfig, s.stream)
	if err != nil {
		return nil, fmt.Errorf("unable to create streaming server")
	}

	// TODO: Is it should be started somewhere else?
	go func() {
		s.stream.streamServer.Start(true)
	}()

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

// GetSandboxContainer returns the infra container for a given sandbox
func (s *Server) GetSandboxContainer(id string) (*oci.Container, error) {
	sb, err := s.getSandbox(id)
	if err != nil {
		return nil, err
	}

	return sb.infraContainer, nil
}

// GetContainer returns a container by its ID
func (s *Server) GetContainer(id string) (*oci.Container, error) {
	return s.getContainer(id)
}

func (s *Server) removeContainer(c *oci.Container) error {
	return s.state.DeleteContainer(c.ID(), c.Sandbox())
}
