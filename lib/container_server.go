package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/containers/image/types"
	"github.com/containers/libpod/pkg/hooks"
	cstorage "github.com/containers/storage"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/truncindex"
	"github.com/kubernetes-sigs/cri-o/lib/sandbox"
	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/kubernetes-sigs/cri-o/pkg/annotations"
	"github.com/kubernetes-sigs/cri-o/pkg/registrar"
	"github.com/kubernetes-sigs/cri-o/pkg/storage"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/text/language"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

var (
	// localeToLanguage maps from locale values to language tags.
	localeToLanguage = map[string]string{
		"":      "und-u-va-posix",
		"c":     "und-u-va-posix",
		"posix": "und-u-va-posix",
	}
)

// ContainerServer implements the ImageServer
type ContainerServer struct {
	runtime              *oci.Runtime
	store                cstorage.Store
	storageImageServer   storage.ImageServer
	storageRuntimeServer storage.RuntimeServer
	updateLock           sync.RWMutex
	ctrNameIndex         *registrar.Registrar
	ctrIDIndex           *truncindex.TruncIndex
	podNameIndex         *registrar.Registrar
	podIDIndex           *truncindex.TruncIndex
	Hooks                *hooks.Manager

	imageContext *types.SystemContext
	stateLock    sync.Locker
	state        *containerServerState
	config       *Config
}

// Runtime returns the oci runtime for the ContainerServer
func (c *ContainerServer) Runtime() *oci.Runtime {
	return c.runtime
}

// Store returns the Store for the ContainerServer
func (c *ContainerServer) Store() cstorage.Store {
	return c.store
}

// StorageImageServer returns the ImageServer for the ContainerServer
func (c *ContainerServer) StorageImageServer() storage.ImageServer {
	return c.storageImageServer
}

// CtrNameIndex returns the Registrar for the ContainerServer
func (c *ContainerServer) CtrNameIndex() *registrar.Registrar {
	return c.ctrNameIndex
}

// CtrIDIndex returns the TruncIndex for the ContainerServer
func (c *ContainerServer) CtrIDIndex() *truncindex.TruncIndex {
	return c.ctrIDIndex
}

// PodNameIndex returns the index of pod names
func (c *ContainerServer) PodNameIndex() *registrar.Registrar {
	return c.podNameIndex
}

// PodIDIndex returns the index of pod IDs
func (c *ContainerServer) PodIDIndex() *truncindex.TruncIndex {
	return c.podIDIndex
}

// ImageContext returns the SystemContext for the ContainerServer
func (c *ContainerServer) ImageContext() *types.SystemContext {
	return c.imageContext
}

// Config gets the configuration for the ContainerServer
func (c *ContainerServer) Config() *Config {
	return c.config
}

// StorageRuntimeServer gets the runtime server for the ContainerServer
func (c *ContainerServer) StorageRuntimeServer() storage.RuntimeServer {
	return c.storageRuntimeServer
}

// New creates a new ContainerServer with options provided
func New(ctx context.Context, config *Config) (*ContainerServer, error) {
	store, err := cstorage.GetStore(cstorage.StoreOptions{
		RunRoot:            config.RunRoot,
		GraphRoot:          config.Root,
		GraphDriverName:    config.Storage,
		GraphDriverOptions: config.StorageOptions,
	})
	if err != nil {
		return nil, err
	}

	imageService, err := storage.GetImageService(ctx, store, config.DefaultTransport, config.InsecureRegistries, config.Registries)
	if err != nil {
		return nil, err
	}

	storageRuntimeService := storage.GetRuntimeService(ctx, imageService, config.PauseImage)
	if err != nil {
		return nil, err
	}

	runtime, err := oci.New(config.Runtime, config.RuntimeUntrustedWorkload, config.DefaultWorkloadTrust, config.Runtimes, config.Conmon, config.ConmonEnv, config.CgroupManager, config.ContainerExitsDir, config.ContainerAttachSocketDir, config.LogSizeMax, config.NoPivot, config.CtrStopTimeout)
	if err != nil {
		return nil, err
	}

	var lock sync.Locker
	if config.FileLocking {
		fileLock, err := cstorage.GetLockfile(config.FileLockingPath)
		if err != nil {
			return nil, fmt.Errorf("error obtaining lockfile: %v", err)
		}
		lock = fileLock
	} else {
		lock = new(sync.Mutex)
	}

	hookDirectories := []string{}
	missingHookDirectoryFatal := false

	// If hooks directory is set in config use it
	if config.HooksDirPath != "" {
		hookDirectories = append(hookDirectories, config.HooksDirPath)

		// If user overrode default hooks, this means it is in a test, so don't
		// use OverrideHooksDirPath
		if config.HooksDirPath == hooks.DefaultDir {
			hookDirectories = append(hookDirectories, hooks.OverrideDir)
		} else {
			missingHookDirectoryFatal = true
		}
	}

	var locale string
	var ok bool
	for _, envVar := range []string{
		"LC_ALL",
		"LC_COLLATE",
		"LANG",
	} {
		locale, ok = os.LookupEnv(envVar)
		if ok {
			break
		}
	}

	langString, ok := localeToLanguage[strings.ToLower(locale)]
	if !ok {
		langString = locale
	}

	lang, err := language.Parse(langString)
	if err != nil {
		logrus.Warnf("failed to parse language %q: %s", langString, err)
		lang, err = language.Parse("und-u-va-posix")
		if err != nil {
			return nil, err
		}
	}

	hooks, err := hooks.New(ctx, hookDirectories, []string{}, lang)
	if err != nil {
		if missingHookDirectoryFatal || !os.IsNotExist(err) {
			return nil, err
		}
		logrus.Warnf("failed to load hooks: %v", err)
	}

	return &ContainerServer{
		runtime:              runtime,
		store:                store,
		storageImageServer:   imageService,
		storageRuntimeServer: storageRuntimeService,
		ctrNameIndex:         registrar.NewRegistrar(),
		ctrIDIndex:           truncindex.NewTruncIndex([]string{}),
		podNameIndex:         registrar.NewRegistrar(),
		podIDIndex:           truncindex.NewTruncIndex([]string{}),
		imageContext:         &types.SystemContext{SignaturePolicyPath: config.SignaturePolicyPath},
		Hooks:                hooks,
		stateLock:            lock,
		state: &containerServerState{
			containers:      oci.NewMemoryStore(),
			infraContainers: oci.NewMemoryStore(),
			sandboxes:       sandbox.NewMemoryStore(),
			processLevels:   make(map[string]int),
		},
		config: config,
	}, nil
}

// Update makes changes to the server's state (lists of pods and containers) to
// reflect the list of pods and containers that are stored on disk, possibly
// having been modified by other parties
func (c *ContainerServer) Update() error {
	c.updateLock.Lock()
	defer c.updateLock.Unlock()

	containers, err := c.store.Containers()
	if err != nil && !os.IsNotExist(errors.Cause(err)) {
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
		if c.HasSandbox(container.ID) {
			// FIXME: do we need to reload/update any info about the sandbox?
			oldPods[container.ID] = container.ID
			oldPodContainers[container.ID] = container.ID
			continue
		}
		if c.GetContainer(container.ID) != nil {
			// FIXME: do we need to reload/update any info about the container?
			oldPodContainers[container.ID] = container.ID
			continue
		}
		// not previously known, so figure out what it is
		metadata, err2 := c.storageRuntimeServer.GetContainerMetadata(container.ID)
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
	c.ctrIDIndex.Iterate(func(id string) {
		if _, ok := oldPodContainers[id]; !ok {
			// this container's ID wasn't in the updated list -> removed
			removedPodContainers[id] = id
		} else {
			ctr := c.GetContainer(id)
			if ctr != nil {
				// if the container exists, update its state
				c.ContainerStateFromDisk(c.GetContainer(id))
			}
		}
	})
	for removedPodContainer := range removedPodContainers {
		// forget this container
		ctr := c.GetContainer(removedPodContainer)
		if ctr == nil {
			logrus.Warnf("bad state when getting container removed %+v", removedPodContainer)
			continue
		}
		c.ReleaseContainerName(ctr.Name())
		c.RemoveContainer(ctr)
		if err = c.ctrIDIndex.Delete(ctr.ID()); err != nil {
			return err
		}
		logrus.Debugf("forgetting removed pod container %s", ctr.ID())
	}
	c.PodIDIndex().Iterate(func(id string) {
		if _, ok := oldPods[id]; !ok {
			// this pod's ID wasn't in the updated list -> removed
			removedPods[id] = id
		}
	})
	for removedPod := range removedPods {
		// forget this pod
		sb := c.GetSandbox(removedPod)
		if sb == nil {
			logrus.Warnf("bad state when getting pod to remove %+v", removedPod)
			continue
		}
		podInfraContainer := sb.InfraContainer()
		c.ReleaseContainerName(podInfraContainer.Name())
		c.RemoveContainer(podInfraContainer)
		if err = c.ctrIDIndex.Delete(podInfraContainer.ID()); err != nil {
			return err
		}
		sb.RemoveInfraContainer()
		c.ReleasePodName(sb.Name())
		c.RemoveSandbox(sb.ID())
		if err = c.podIDIndex.Delete(sb.ID()); err != nil {
			return err
		}
		logrus.Debugf("forgetting removed pod %s", sb.ID())
	}
	for sandboxID := range newPods {
		// load this pod
		if err = c.LoadSandbox(sandboxID); err != nil {
			logrus.Warnf("could not load new pod sandbox %s: %v, ignoring", sandboxID, err)
		} else {
			logrus.Debugf("loaded new pod sandbox %s: %v", sandboxID, err)
		}
	}
	for containerID := range newPodContainers {
		// load this container
		if err = c.LoadContainer(containerID); err != nil {
			logrus.Warnf("could not load new sandbox container %s: %v, ignoring", containerID, err)
		} else {
			logrus.Debugf("loaded new pod container %s: %v", containerID, err)
		}
	}
	return nil
}

// LoadSandbox loads a sandbox from the disk into the sandbox store
func (c *ContainerServer) LoadSandbox(id string) error {
	config, err := c.store.FromContainerDirectory(id, "config.json")
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
	name, err = c.ReservePodName(id, name)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			c.ReleasePodName(name)
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

	spp := m.Annotations[annotations.SeccompProfilePath]

	kubeAnnotations := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Annotations]), &kubeAnnotations); err != nil {
		return err
	}

	portMappings := []*hostport.PortMapping{}
	if err := json.Unmarshal([]byte(m.Annotations[annotations.PortMappings]), &portMappings); err != nil {
		return err
	}

	privileged := isTrue(m.Annotations[annotations.PrivilegedRuntime])
	trusted := isTrue(m.Annotations[annotations.TrustedSandbox])
	hostNetwork := isTrue(m.Annotations[annotations.HostNetwork])
	nsOpts := pb.NamespaceOption{}
	if err := json.Unmarshal([]byte(m.Annotations[annotations.NamespaceOptions]), &nsOpts); err != nil {
		return err
	}

	sb, err := sandbox.New(id, m.Annotations[annotations.Namespace], name, m.Annotations[annotations.KubeName], filepath.Dir(m.Annotations[annotations.LogPath]), labels, kubeAnnotations, processLabel, mountLabel, &metadata, m.Annotations[annotations.ShmPath], m.Annotations[annotations.CgroupParent], privileged, trusted, m.Annotations[annotations.RuntimeHandler], m.Annotations[annotations.ResolvPath], m.Annotations[annotations.HostName], portMappings, hostNetwork)
	if err != nil {
		return err
	}
	sb.AddHostnamePath(m.Annotations[annotations.HostnamePath])
	sb.SetSeccompProfilePath(spp)
	sb.SetNamespaceOptions(&nsOpts)

	// We add a netNS only if we can load a permanent one.
	// Otherwise, the sandbox will live in the host namespace.
	if c.config.ManageNetworkNSLifecycle {
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
	}

	c.AddSandbox(sb)

	defer func() {
		if err != nil {
			c.RemoveSandbox(sb.ID())
		}
	}()

	sandboxPath, err := c.store.ContainerRunDirectory(id)
	if err != nil {
		return err
	}

	sandboxDir, err := c.store.ContainerDirectory(id)
	if err != nil {
		return err
	}

	cname, err := c.ReserveContainerName(m.Annotations[annotations.ContainerID], m.Annotations[annotations.ContainerName])
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			c.ReleaseContainerName(cname)
		}
	}()

	created, err := time.Parse(time.RFC3339Nano, m.Annotations[annotations.Created])
	if err != nil {
		return err
	}

	scontainer, err := oci.NewContainer(m.Annotations[annotations.ContainerID], cname, sandboxPath, m.Annotations[annotations.LogPath], sb.NetNs().Path(), labels, m.Annotations, kubeAnnotations, "", "", "", nil, id, false, false, false, privileged, trusted, sb.RuntimeHandler(), sandboxDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
	if err != nil {
		return err
	}
	scontainer.SetSpec(&m)
	scontainer.SetMountPoint(m.Annotations[annotations.MountPoint])

	if m.Annotations[annotations.Volumes] != "" {
		containerVolumes := []oci.ContainerVolume{}
		if err = json.Unmarshal([]byte(m.Annotations[annotations.Volumes]), &containerVolumes); err != nil {
			return fmt.Errorf("failed to unmarshal container volumes: %v", err)
		}
		for _, cv := range containerVolumes {
			scontainer.AddVolume(cv)
		}
	}

	c.ContainerStateFromDisk(scontainer)
	sb.SetCreated()

	if err = label.ReserveLabel(processLabel); err != nil {
		return err
	}
	sb.SetInfraContainer(scontainer)
	if err = c.ctrIDIndex.Add(scontainer.ID()); err != nil {
		return err
	}
	if err = c.podIDIndex.Add(id); err != nil {
		return err
	}
	return nil
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

// LoadContainer loads a container from the disk into the container store
func (c *ContainerServer) LoadContainer(id string) error {
	config, err := c.store.FromContainerDirectory(id, "config.json")
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
	name, err = c.ReserveContainerName(id, name)
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			c.ReleaseContainerName(name)
		}
	}()

	var metadata pb.ContainerMetadata
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Metadata]), &metadata); err != nil {
		return err
	}
	sb := c.GetSandbox(m.Annotations[annotations.SandboxID])
	if sb == nil {
		return fmt.Errorf("could not get sandbox with id %s, skipping", m.Annotations[annotations.SandboxID])
	}

	tty := isTrue(m.Annotations[annotations.TTY])
	stdin := isTrue(m.Annotations[annotations.Stdin])
	stdinOnce := isTrue(m.Annotations[annotations.StdinOnce])

	containerPath, err := c.store.ContainerRunDirectory(id)
	if err != nil {
		return err
	}

	containerDir, err := c.store.ContainerDirectory(id)
	if err != nil {
		return err
	}

	img, ok := m.Annotations[annotations.Image]
	if !ok {
		img = ""
	}

	imgName, ok := m.Annotations[annotations.ImageName]
	if !ok {
		imgName = ""
	}

	imgRef, ok := m.Annotations[annotations.ImageRef]
	if !ok {
		imgRef = ""
	}

	kubeAnnotations := make(map[string]string)
	if err = json.Unmarshal([]byte(m.Annotations[annotations.Annotations]), &kubeAnnotations); err != nil {
		return err
	}

	created, err := time.Parse(time.RFC3339Nano, m.Annotations[annotations.Created])
	if err != nil {
		return err
	}

	ctr, err := oci.NewContainer(id, name, containerPath, m.Annotations[annotations.LogPath], sb.NetNs().Path(), labels, m.Annotations, kubeAnnotations, img, imgName, imgRef, &metadata, sb.ID(), tty, stdin, stdinOnce, sb.Privileged(), sb.Trusted(), sb.RuntimeHandler(), containerDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
	if err != nil {
		return err
	}
	ctr.SetSpec(&m)
	ctr.SetMountPoint(m.Annotations[annotations.MountPoint])
	spp := m.Annotations[annotations.SeccompProfilePath]
	ctr.SetSeccompProfilePath(spp)

	c.ContainerStateFromDisk(ctr)
	ctr.SetCreated()

	c.AddContainer(ctr)
	return c.ctrIDIndex.Add(id)
}

func isTrue(annotaton string) bool {
	return annotaton == "true"
}

// ContainerStateFromDisk retrieves information on the state of a running container
// from the disk
func (c *ContainerServer) ContainerStateFromDisk(ctr *oci.Container) error {
	if err := ctr.FromDisk(); err != nil {
		return err
	}
	// ignore errors, this is a best effort to have up-to-date info about
	// a given container before its state gets stored
	c.runtime.UpdateStatus(ctr)

	return nil
}

// ContainerStateToDisk writes the container's state information to a JSON file
// on disk
func (c *ContainerServer) ContainerStateToDisk(ctr *oci.Container) error {
	// ignore errors, this is a best effort to have up-to-date info about
	// a given container before its state gets stored
	c.Runtime().UpdateStatus(ctr)

	jsonSource, err := ioutils.NewAtomicFileWriter(ctr.StatePath(), 0644)
	if err != nil {
		return err
	}
	defer jsonSource.Close()
	enc := json.NewEncoder(jsonSource)
	return enc.Encode(c.runtime.ContainerStatus(ctr))
}

// ReserveContainerName holds a name for a container that is being created
func (c *ContainerServer) ReserveContainerName(id, name string) (string, error) {
	if err := c.ctrNameIndex.Reserve(name, id); err != nil {
		if err == registrar.ErrNameReserved {
			id, err := c.ctrNameIndex.Get(name)
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

// ReleaseContainerName releases a container name from the index so that it can
// be used by other containers
func (c *ContainerServer) ReleaseContainerName(name string) {
	c.ctrNameIndex.Release(name)
}

// ReservePodName holds a name for a pod that is being created
func (c *ContainerServer) ReservePodName(id, name string) (string, error) {
	if err := c.podNameIndex.Reserve(name, id); err != nil {
		if err == registrar.ErrNameReserved {
			id, err := c.podNameIndex.Get(name)
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

// ReleasePodName releases a pod name from the index so it can be used by other
// pods
func (c *ContainerServer) ReleasePodName(name string) {
	c.podNameIndex.Release(name)
}

// Shutdown attempts to shut down the server's storage cleanly
func (c *ContainerServer) Shutdown() error {
	_, err := c.store.Shutdown(false)
	if err != nil && errors.Cause(err) != cstorage.ErrLayerUsedByContainer {
		return err
	}
	return nil
}

type containerServerState struct {
	containers      oci.ContainerStorer
	infraContainers oci.ContainerStorer
	sandboxes       sandbox.Storer
	// processLevels The number of sandboxes using the same SELinux MCS level. Need to release MCS Level, when count reaches 0
	processLevels map[string]int
}

// AddContainer adds a container to the container state store
func (c *ContainerServer) AddContainer(ctr *oci.Container) {
	sandbox := c.state.sandboxes.Get(ctr.Sandbox())
	sandbox.AddContainer(ctr)
	c.state.containers.Add(ctr.ID(), ctr)
}

// AddInfraContainer adds a container to the container state store
func (c *ContainerServer) AddInfraContainer(ctr *oci.Container) {
	c.state.infraContainers.Add(ctr.ID(), ctr)
}

// GetContainer returns a container by its ID
func (c *ContainerServer) GetContainer(id string) *oci.Container {
	return c.state.containers.Get(id)
}

// GetInfraContainer returns a container by its ID
func (c *ContainerServer) GetInfraContainer(id string) *oci.Container {
	return c.state.infraContainers.Get(id)
}

// HasContainer checks if a container exists in the state
func (c *ContainerServer) HasContainer(id string) bool {
	return c.state.containers.Get(id) != nil
}

// RemoveContainer removes a container from the container state store
func (c *ContainerServer) RemoveContainer(ctr *oci.Container) {
	sbID := ctr.Sandbox()
	sb := c.state.sandboxes.Get(sbID)
	sb.RemoveContainer(ctr)
	c.state.containers.Delete(ctr.ID())
}

// RemoveInfraContainer removes a container from the container state store
func (c *ContainerServer) RemoveInfraContainer(ctr *oci.Container) {
	c.state.infraContainers.Delete(ctr.ID())
}

// listContainers returns a list of all containers stored by the server state
func (c *ContainerServer) listContainers() []*oci.Container {
	return c.state.containers.List()
}

// ListContainers returns a list of all containers stored by the server state
// that match the given filter function
func (c *ContainerServer) ListContainers(filters ...func(*oci.Container) bool) ([]*oci.Container, error) {
	containers := c.listContainers()
	if len(filters) == 0 {
		return containers, nil
	}
	filteredContainers := make([]*oci.Container, 0, len(containers))
	for _, container := range containers {
		for _, filter := range filters {
			if filter(container) {
				filteredContainers = append(filteredContainers, container)
			}
		}
	}
	return filteredContainers, nil
}

// AddSandbox adds a sandbox to the sandbox state store
func (c *ContainerServer) AddSandbox(sb *sandbox.Sandbox) {
	c.state.sandboxes.Add(sb.ID(), sb)

	c.stateLock.Lock()
	c.addSandboxPlatform(sb)
	c.stateLock.Unlock()
}

// GetSandbox returns a sandbox by its ID
func (c *ContainerServer) GetSandbox(id string) *sandbox.Sandbox {
	return c.state.sandboxes.Get(id)
}

// GetSandboxContainer returns a sandbox's infra container
func (c *ContainerServer) GetSandboxContainer(id string) *oci.Container {
	sb := c.state.sandboxes.Get(id)
	return sb.InfraContainer()
}

// HasSandbox checks if a sandbox exists in the state
func (c *ContainerServer) HasSandbox(id string) bool {
	return c.state.sandboxes.Get(id) != nil
}

// RemoveSandbox removes a sandbox from the state store
func (c *ContainerServer) RemoveSandbox(id string) {
	sb := c.state.sandboxes.Get(id)

	c.stateLock.Lock()
	c.removeSandboxPlatform(sb)
	c.stateLock.Unlock()

	c.state.sandboxes.Delete(id)
}

// ListSandboxes lists all sandboxes in the state store
func (c *ContainerServer) ListSandboxes() []*sandbox.Sandbox {
	return c.state.sandboxes.List()
}
