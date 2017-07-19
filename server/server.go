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
	cstorage "github.com/containers/storage"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/registrar"
	"github.com/docker/docker/pkg/truncindex"
	"github.com/kubernetes-incubator/cri-o/libkpod"
	"github.com/kubernetes-incubator/cri-o/libkpod/sandbox"
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
	libkpod.ContainerServer
	config               Config
	storageRuntimeServer storage.RuntimeServer
	stateLock            sync.Locker
	updateLock           sync.RWMutex
	state                *serverState
	netPlugin            ocicni.CNIPlugin
	hostportManager      hostport.HostPortManager
	podNameIndex         *registrar.Registrar
	podIDIndex           *truncindex.TruncIndex

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
	config, err := s.Store().FromContainerDirectory(id, "config.json")
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
	name, err = s.reserveContainerName(id, name)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			s.releaseContainerName(name)
		}
	}()

	var metadata pb.ContainerMetadata
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Metadata]), &metadata); err != nil {
		return err
	}
	sb := s.getSandbox(m.Annotations[annotations.SandboxID])
	if sb == nil {
		return fmt.Errorf("could not get sandbox with id %s, skipping", m.Annotations[annotations.SandboxID])
	}

	tty := isTrue(m.Annotations[annotations.TTY])
	stdin := isTrue(m.Annotations[annotations.Stdin])
	stdinOnce := isTrue(m.Annotations[annotations.StdinOnce])

	containerPath, err := s.Store().ContainerRunDirectory(id)
	if err != nil {
		return err
	}

	containerDir, err := s.Store().ContainerDirectory(id)
	if err != nil {
		return err
	}

	img, ok := m.Annotations[annotations.Image]
	if !ok {
		img = ""
	}

	kubeAnnotations := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Annotations]), &kubeAnnotations); err != nil {
		return err
	}

	created, err := time.Parse(time.RFC3339Nano, m.Annotations[annotations.Created])
	if err != nil {
		return err
	}

	ctr, err := oci.NewContainer(id, name, containerPath, m.Annotations[annotations.LogPath], sb.NetNs(), labels, kubeAnnotations, img, &metadata, sb.ID(), tty, stdin, stdinOnce, sb.Privileged(), sb.Trusted(), containerDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
	if err != nil {
		return err
	}

	s.containerStateFromDisk(ctr)

	s.addContainer(ctr)
	return s.CtrIDIndex().Add(id)
}

func (s *Server) containerStateFromDisk(c *oci.Container) error {
	if err := c.FromDisk(); err != nil {
		return err
	}
	// ignore errors, this is a best effort to have up-to-date info about
	// a given container before its state gets stored
	s.Runtime().UpdateStatus(c)

	return nil
}

func (s *Server) containerStateToDisk(c *oci.Container) error {
	// ignore errors, this is a best effort to have up-to-date info about
	// a given container before its state gets stored
	s.Runtime().UpdateStatus(c)

	jsonSource, err := ioutils.NewAtomicFileWriter(c.StatePath(), 0644)
	if err != nil {
		return err
	}
	defer jsonSource.Close()
	enc := json.NewEncoder(jsonSource)
	return enc.Encode(s.Runtime().ContainerStatus(c))
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
	config, err := s.Store().FromContainerDirectory(id, "config.json")
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
	name, err = s.reservePodName(id, name)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			s.releasePodName(name)
		}
	}()
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

	sb, err := sandbox.New(id, name, m.Annotations[annotations.KubeName], filepath.Dir(m.Annotations[annotations.LogPath]), "", labels, kubeAnnotations, processLabel, mountLabel, &metadata, m.Annotations[annotations.ShmPath], "", privileged, trusted, m.Annotations[annotations.ResolvPath], "", nil)
	if err != nil {
		return err
	}

	// We add a netNS only if we can load a permanent one.
	// Otherwise, the sandbox will live in the host namespace.
	netNsPath, err := configNetNsPath(m)
	if err == nil {
		nsErr := sb.NetNsJoin(netNsPath, sb.Name())
		// If we can't load the networking namespace
		// because it's closed, we just set the sb netns
		// pointer to nil. Otherwise we return an error.
		if nsErr != nil && nsErr != sandbox.ErrClosedNetNS {
			return nsErr
		}
	}

	s.addSandbox(sb)

	defer func() {
		if err != nil {
			s.removeSandbox(sb.ID())
		}
	}()

	sandboxPath, err := s.Store().ContainerRunDirectory(id)
	if err != nil {
		return err
	}

	sandboxDir, err := s.Store().ContainerDirectory(id)
	if err != nil {
		return err
	}

	cname, err := s.reserveContainerName(m.Annotations[annotations.ContainerID], m.Annotations[annotations.ContainerName])
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			s.releaseContainerName(cname)
		}
	}()

	created, err := time.Parse(time.RFC3339Nano, m.Annotations[annotations.Created])
	if err != nil {
		return err
	}

	scontainer, err := oci.NewContainer(m.Annotations[annotations.ContainerID], cname, sandboxPath, m.Annotations[annotations.LogPath], sb.NetNs(), labels, kubeAnnotations, "", nil, id, false, false, false, privileged, trusted, sandboxDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
	if err != nil {
		return err
	}

	s.containerStateFromDisk(scontainer)

	if err = label.ReserveLabel(processLabel); err != nil {
		return err
	}
	sb.SetInfraContainer(scontainer)
	if err = s.CtrIDIndex().Add(scontainer.ID()); err != nil {
		return err
	}
	if err = s.podIDIndex.Add(id); err != nil {
		return err
	}
	return nil
}

func (s *Server) restore() {
	containers, err := s.Store().Containers()
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

	containers, err := s.Store().Containers()
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
		if s.getContainer(container.ID) != nil {
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
	s.CtrIDIndex().Iterate(func(id string) {
		if _, ok := oldPodContainers[id]; !ok {
			// this container's ID wasn't in the updated list -> removed
			removedPodContainers[id] = id
		}
	})
	for removedPodContainer := range removedPodContainers {
		// forget this container
		c := s.getContainer(removedPodContainer)
		if c == nil {
			logrus.Warnf("bad state when getting container removed %+v", removedPodContainer)
			continue
		}
		s.releaseContainerName(c.Name())
		s.removeContainer(c)
		if err = s.CtrIDIndex().Delete(c.ID()); err != nil {
			return err
		}
		logrus.Debugf("forgetting removed pod container %s", c.ID())
	}
	s.podIDIndex.Iterate(func(id string) {
		if _, ok := oldPods[id]; !ok {
			// this pod's ID wasn't in the updated list -> removed
			removedPods[id] = id
		}
	})
	for removedPod := range removedPods {
		// forget this pod
		sb := s.getSandbox(removedPod)
		if sb == nil {
			logrus.Warnf("bad state when getting pod to remove %+v", removedPod)
			continue
		}
		podInfraContainer := sb.InfraContainer()
		s.releaseContainerName(podInfraContainer.Name())
		s.removeContainer(podInfraContainer)
		if err = s.CtrIDIndex().Delete(podInfraContainer.ID()); err != nil {
			return err
		}
		sb.RemoveInfraContainer()
		s.releasePodName(sb.Name())
		s.removeSandbox(sb.ID())
		if err = s.podIDIndex.Delete(sb.ID()); err != nil {
			return err
		}
		logrus.Debugf("forgetting removed pod %s", sb.ID())
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

func (s *Server) reservePodName(id, name string) (string, error) {
	if err := s.podNameIndex.Reserve(name, id); err != nil {
		if err == registrar.ErrNameReserved {
			id, err := s.podNameIndex.Get(name)
			if err != nil {
				logrus.Warnf("conflict, pod name %q already reserved", name)
				return "", err
			}
			return "", fmt.Errorf("conflict, name %q already reserved for pod %q", name, id)
		}
		return "", fmt.Errorf("error reserving pod name %q", name)
	}
	return name, nil
}

func (s *Server) releasePodName(name string) {
	s.podNameIndex.Release(name)
}

func (s *Server) reserveContainerName(id, name string) (string, error) {
	if err := s.CtrNameIndex().Reserve(name, id); err != nil {
		if err == registrar.ErrNameReserved {
			id, err := s.CtrNameIndex().Get(name)
			if err != nil {
				logrus.Warnf("conflict, ctr name %q already reserved", name)
				return "", err
			}
			return "", fmt.Errorf("conflict, name %q already reserved for ctr %q", name, id)
		}
		return "", fmt.Errorf("error reserving ctr name %s", name)
	}
	return name, nil
}

func (s *Server) releaseContainerName(name string) {
	s.CtrNameIndex().Release(name)
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
	// why do this on clean shutdown! we want containers left running when crio
	// is down for whatever reason no?!
	// notice this won't trigger just on system halt but also on normal
	// crio.service restart!!!
	s.cleanupSandboxesOnShutdown()
	_, err := s.Store().Shutdown(false)
	return err
}

// New creates a new Server with options provided
func New(config *Config) (*Server, error) {
	store, err := cstorage.GetStore(cstorage.StoreOptions{
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

	sandboxes := make(map[string]*sandbox.Sandbox)
	netPlugin, err := ocicni.InitCNI(config.NetworkDir, config.PluginDir)
	if err != nil {
		return nil, err
	}
	iptInterface := utiliptables.New(utilexec.New(), utildbus.New(), utiliptables.ProtocolIpv4)
	iptInterface.EnsureChain(utiliptables.TableNAT, iptablesproxy.KubeMarkMasqChain)
	hostportManager := hostport.NewHostportManager()

	containerServer := libkpod.New(r, store, imageService, registrar.NewRegistrar(), truncindex.NewTruncIndex([]string{}), &types.SystemContext{SignaturePolicyPath: config.ImageConfig.SignaturePolicyPath})
	s := &Server{
		ContainerServer:      *containerServer,
		storageRuntimeServer: storageRuntimeService,
		stateLock:            new(sync.Mutex),
		netPlugin:            netPlugin,
		hostportManager:      hostportManager,
		config:               *config,
		state: &serverState{
			sandboxes: sandboxes,
		},
		seccompEnabled:  seccomp.IsEnabled(),
		appArmorEnabled: apparmor.IsEnabled(),
		appArmorProfile: config.ApparmorProfile,
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

	s.podIDIndex = truncindex.NewTruncIndex([]string{})
	s.podNameIndex = registrar.NewRegistrar()

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

	logrus.Debugf("sandboxes: %v", s.state.sandboxes)
	logrus.Debugf("containers: %v", s.ContainerServer.ListContainers())
	return s, nil
}

type serverState struct {
	sandboxes map[string]*sandbox.Sandbox
}

func (s *Server) addSandbox(sb *sandbox.Sandbox) {
	s.stateLock.Lock()
	s.state.sandboxes[sb.ID()] = sb
	s.stateLock.Unlock()
}

func (s *Server) getSandbox(id string) *sandbox.Sandbox {
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

func (s *Server) removeSandbox(id string) {
	s.stateLock.Lock()
	delete(s.state.sandboxes, id)
	s.stateLock.Unlock()
}

func (s *Server) addContainer(c *oci.Container) {
	s.stateLock.Lock()
	sandbox := s.state.sandboxes[c.Sandbox()]
	// TODO(runcom): handle !ok above!!! otherwise it panics!
	sandbox.AddContainer(c)
	s.ContainerServer.AddContainer(c)
	s.stateLock.Unlock()
}

func (s *Server) getContainer(id string) *oci.Container {
	s.stateLock.Lock()
	c := s.ContainerServer.GetContainer(id)
	s.stateLock.Unlock()
	return c
}

// GetSandboxContainer returns the infra container for a given sandbox
func (s *Server) GetSandboxContainer(id string) *oci.Container {
	sb := s.getSandbox(id)
	return sb.InfraContainer()
}

// GetContainer returns a container by its ID
func (s *Server) GetContainer(id string) *oci.Container {
	return s.getContainer(id)
}

func (s *Server) removeContainer(c *oci.Container) {
	s.stateLock.Lock()
	sandbox := s.state.sandboxes[c.Sandbox()]
	sandbox.RemoveContainer(c)
	s.ContainerServer.RemoveContainer(c)
	s.stateLock.Unlock()
}

func (s *Server) getPodSandboxFromRequest(podSandboxID string) (*sandbox.Sandbox, error) {
	if podSandboxID == "" {
		return nil, sandbox.ErrIDEmpty
	}

	sandboxID, err := s.podIDIndex.Get(podSandboxID)
	if err != nil {
		return nil, fmt.Errorf("PodSandbox with ID starting with %s not found: %v", podSandboxID, err)
	}

	sb := s.getSandbox(sandboxID)
	if sb == nil {
		return nil, fmt.Errorf("specified pod sandbox not found: %s", sandboxID)
	}
	return sb, nil
}
