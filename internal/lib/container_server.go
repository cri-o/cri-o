package lib

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/containers/image/v5/types"
	"github.com/containers/libpod/pkg/annotations"
	"github.com/containers/libpod/pkg/hooks"
	"github.com/containers/libpod/pkg/registrar"
	cstorage "github.com/containers/storage"
	libconfig "github.com/cri-o/cri-o/internal/lib/config"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/pkg/storage"
	"github.com/docker/docker/pkg/ioutils"
	"github.com/docker/docker/pkg/truncindex"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

// ContainerServer implements the ImageServer
type ContainerServer struct {
	runtime              oci.RuntimeImpl
	store                cstorage.Store
	storageImageServer   storage.ImageServer
	storageRuntimeServer storage.RuntimeServer
	updateLock           sync.RWMutex
	ctrNameIndex         *registrar.Registrar
	ctrIDIndex           *truncindex.TruncIndex
	podNameIndex         *registrar.Registrar
	podIDIndex           *truncindex.TruncIndex
	Hooks                *hooks.Manager

	stateLock sync.Locker
	state     *containerServerState
	config    *libconfig.Config
}

// Runtime returns the oci runtime for the ContainerServer
func (c *ContainerServer) Runtime() oci.RuntimeImpl {
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

// Config gets the configuration for the ContainerServer
func (c *ContainerServer) Config() *libconfig.Config {
	return c.config
}

// StorageRuntimeServer gets the runtime server for the ContainerServer
func (c *ContainerServer) StorageRuntimeServer() storage.RuntimeServer {
	return c.storageRuntimeServer
}

// New creates a new ContainerServer with options provided
func New(ctx context.Context, systemContext *types.SystemContext, configIface libconfig.Iface) (*ContainerServer, error) {
	if configIface == nil {
		return nil, fmt.Errorf("provided config is nil")
	}
	store, err := configIface.GetStore()
	if err != nil {
		return nil, err
	}
	config := configIface.GetData()

	if config == nil {
		return nil, fmt.Errorf("cannot create container server: interface is nil")
	}

	imageService, err := storage.GetImageService(ctx, systemContext, store, config.DefaultTransport, config.InsecureRegistries, config.Registries)
	if err != nil {
		return nil, err
	}

	storageRuntimeService := storage.GetRuntimeService(ctx, imageService)

	runtime := oci.New(config)

	newHooks, err := hooks.New(ctx, config.HooksDir, []string{})
	if err != nil {
		return nil, err
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
		Hooks:                newHooks,
		stateLock:            &sync.Mutex{},
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
	for i := range containers {
		container := &containers[i]
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
				if err := c.ContainerStateFromDisk(c.GetContainer(id)); err != nil {
					logrus.Warnf("unable to retrieve containers %s state from disk: %v", id, err)
				}
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
		if err := c.ctrIDIndex.Delete(ctr.ID()); err != nil {
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
		if err := c.ctrIDIndex.Delete(podInfraContainer.ID()); err != nil {
			return err
		}
		sb.RemoveInfraContainer()
		c.ReleasePodName(sb.Name())
		if err := c.RemoveSandbox(sb.ID()); err != nil {
			logrus.Warnf("failed to remove sandbox ID %s: %v", sb.ID(), err)
		}
		if err := c.podIDIndex.Delete(sb.ID()); err != nil {
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
	if err := json.Unmarshal(config, &m); err != nil {
		return err
	}
	labels := make(map[string]string)
	if err := json.Unmarshal([]byte(m.Annotations[annotations.Labels]), &labels); err != nil {
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
	if err := json.Unmarshal([]byte(m.Annotations[annotations.Metadata]), &metadata); err != nil {
		return err
	}

	dupLabels, err := label.DupSecOpt(m.Process.SelinuxLabel)
	if err != nil {
		return err
	}

	processLabel, mountLabel, err := label.InitLabels(dupLabels)
	if err != nil {
		return err
	}

	spp := m.Annotations[annotations.SeccompProfilePath]

	kubeAnnotations := make(map[string]string)
	if err := json.Unmarshal([]byte(m.Annotations[annotations.Annotations]), &kubeAnnotations); err != nil {
		return err
	}

	portMappings := []*hostport.PortMapping{}
	if err := json.Unmarshal([]byte(m.Annotations[annotations.PortMappings]), &portMappings); err != nil {
		return err
	}

	privileged := isTrue(m.Annotations[annotations.PrivilegedRuntime])
	hostNetwork := isTrue(m.Annotations[annotations.HostNetwork])
	nsOpts := pb.NamespaceOption{}
	if err := json.Unmarshal([]byte(m.Annotations[annotations.NamespaceOptions]), &nsOpts); err != nil {
		return err
	}

	sb, err := sandbox.New(id, m.Annotations[annotations.Namespace], name, m.Annotations[annotations.KubeName], filepath.Dir(m.Annotations[annotations.LogPath]), labels, kubeAnnotations, processLabel, mountLabel, &metadata, m.Annotations[annotations.ShmPath], m.Annotations[annotations.CgroupParent], privileged, m.Annotations[annotations.RuntimeHandler], m.Annotations[annotations.ResolvPath], m.Annotations[annotations.HostName], portMappings, hostNetwork)
	if err != nil {
		return err
	}
	sb.AddHostnamePath(m.Annotations[annotations.HostnamePath])
	sb.SetSeccompProfilePath(spp)
	sb.SetNamespaceOptions(&nsOpts)

	// We add a netNS only if we can load a permanent one.
	// Otherwise, the sandbox will live in the host namespace.
	if c.config.ManageNetworkNSLifecycle {
		netNsPath, err := configNetNsPath(&m)
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

	if err := c.AddSandbox(sb); err != nil {
		return err
	}

	defer func() {
		if err != nil {
			if err := c.RemoveSandbox(sb.ID()); err != nil {
				logrus.Warnf("could not remove sandbox ID %s: %v", sb.ID(), err)
			}
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

	scontainer, err := oci.NewContainer(m.Annotations[annotations.ContainerID], cname, sandboxPath, m.Annotations[annotations.LogPath], sb.NetNs().Path(), labels, m.Annotations, kubeAnnotations, "", "", "", nil, id, false, false, false, privileged, sb.RuntimeHandler(), sandboxDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
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

	if err := c.ContainerStateFromDisk(scontainer); err != nil {
		return fmt.Errorf("error reading sandbox state from disk %q: %v", scontainer.ID(), err)
	}
	sb.SetCreated()

	if err := label.ReserveLabel(processLabel); err != nil {
		return err
	}
	if err := sb.SetInfraContainer(scontainer); err != nil {
		return err
	}
	if err := c.ctrIDIndex.Add(scontainer.ID()); err != nil {
		return err
	}
	if err := c.podIDIndex.Add(id); err != nil {
		return err
	}
	return nil
}

func configNetNsPath(spec *rspec.Spec) (string, error) {
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
	if err := json.Unmarshal(config, &m); err != nil {
		return err
	}
	labels := make(map[string]string)
	if err := json.Unmarshal([]byte(m.Annotations[annotations.Labels]), &labels); err != nil {
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
	if err := json.Unmarshal([]byte(m.Annotations[annotations.Metadata]), &metadata); err != nil {
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
	if err := json.Unmarshal([]byte(m.Annotations[annotations.Annotations]), &kubeAnnotations); err != nil {
		return err
	}

	created, err := time.Parse(time.RFC3339Nano, m.Annotations[annotations.Created])
	if err != nil {
		return err
	}

	ctr, err := oci.NewContainer(id, name, containerPath, m.Annotations[annotations.LogPath], sb.NetNs().Path(), labels, m.Annotations, kubeAnnotations, img, imgName, imgRef, &metadata, sb.ID(), tty, stdin, stdinOnce, sb.Privileged(), sb.RuntimeHandler(), containerDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
	if err != nil {
		return err
	}
	ctr.SetSpec(&m)
	ctr.SetMountPoint(m.Annotations[annotations.MountPoint])
	spp := m.Annotations[annotations.SeccompProfilePath]
	ctr.SetSeccompProfilePath(spp)

	if err := c.ContainerStateFromDisk(ctr); err != nil {
		return fmt.Errorf("error reading container state from disk %q: %v", ctr.ID(), err)
	}
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
	if err := c.runtime.UpdateContainerStatus(ctr); err != nil {
		return err
	}

	return nil
}

// ContainerStateToDisk writes the container's state information to a JSON file
// on disk
func (c *ContainerServer) ContainerStateToDisk(ctr *oci.Container) error {
	if err := c.Runtime().UpdateContainerStatus(ctr); err != nil {
		logrus.Warnf("error updating the container status %q: %v", ctr.ID(), err)
	}

	jsonSource, err := ioutils.NewAtomicFileWriter(ctr.StatePath(), 0644)
	if err != nil {
		return err
	}
	defer jsonSource.Close()
	enc := json.NewEncoder(jsonSource)
	return enc.Encode(ctr.State())
}

// ReserveContainerName holds a name for a container that is being created
func (c *ContainerServer) ReserveContainerName(id, name string) (string, error) {
	if err := c.ctrNameIndex.Reserve(name, id); err != nil {
		err = fmt.Errorf("error reserving ctr name %s for id %s", name, id)
		logrus.Warn(err)
		return "", err
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
		err = fmt.Errorf("error reserving pod name %s for id %s", name, id)
		logrus.Warn(err)
		return "", err
	}
	return name, nil
}

// ReleasePodName releases a pod name from the index so it can be used by other
// pods
func (c *ContainerServer) ReleasePodName(name string) {
	c.podNameIndex.Release(name)
}

// recoverLogError recovers a runtime panic and logs the returned error if
// existing
func recoverLogError() {
	if err := recover(); err != nil {
		logrus.Error(err)
	}
}

// Shutdown attempts to shut down the server's storage cleanly
func (c *ContainerServer) Shutdown() error {
	defer recoverLogError()
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
	newSandbox := c.state.sandboxes.Get(ctr.Sandbox())
	if newSandbox == nil {
		return
	}
	newSandbox.AddContainer(ctr)
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
	if sb == nil {
		return
	}
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
func (c *ContainerServer) AddSandbox(sb *sandbox.Sandbox) error {
	c.state.sandboxes.Add(sb.ID(), sb)

	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	return c.addSandboxPlatform(sb)
}

// GetSandbox returns a sandbox by its ID
func (c *ContainerServer) GetSandbox(id string) *sandbox.Sandbox {
	return c.state.sandboxes.Get(id)
}

// GetSandboxContainer returns a sandbox's infra container
func (c *ContainerServer) GetSandboxContainer(id string) *oci.Container {
	sb := c.state.sandboxes.Get(id)
	if sb == nil {
		return nil
	}
	return sb.InfraContainer()
}

// HasSandbox checks if a sandbox exists in the state
func (c *ContainerServer) HasSandbox(id string) bool {
	return c.state.sandboxes.Get(id) != nil
}

// RemoveSandbox removes a sandbox from the state store
func (c *ContainerServer) RemoveSandbox(id string) error {
	sb := c.state.sandboxes.Get(id)
	if sb == nil {
		return nil
	}

	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	if err := c.removeSandboxPlatform(sb); err != nil {
		return err
	}

	c.state.sandboxes.Delete(id)
	return nil
}

// ListSandboxes lists all sandboxes in the state store
func (c *ContainerServer) ListSandboxes() []*sandbox.Sandbox {
	return c.state.sandboxes.List()
}
