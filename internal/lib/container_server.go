package lib

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/containers/common/pkg/hooks"
	cstorage "github.com/containers/storage"
	"github.com/containers/storage/pkg/ioutils"
	"github.com/containers/storage/pkg/truncindex"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	statsserver "github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/registrar"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/storage/references"
	"github.com/cri-o/cri-o/pkg/annotations"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	json "github.com/json-iterator/go"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// ContainerManagerCRIO specifies an annotation value which indicates that the
// container has been created by CRI-O. Usually used together with the key
// `io.container.manager`.
const ContainerManagerCRIO = "cri-o"

// ContainerServer implements the ImageServer
type ContainerServer struct {
	runtime              *oci.Runtime
	store                cstorage.Store
	storageImageServer   storage.ImageServer
	storageRuntimeServer storage.RuntimeServer
	ctrNameIndex         *registrar.Registrar
	ctrIDIndex           *truncindex.TruncIndex
	podNameIndex         *registrar.Registrar
	podIDIndex           *truncindex.TruncIndex
	Hooks                *hooks.Manager
	*statsserver.StatsServer

	stateLock sync.Locker
	state     *containerServerState
	config    *libconfig.Config
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

// CtrIDIndex returns the TruncIndex for the ContainerServer
func (c *ContainerServer) CtrIDIndex() *truncindex.TruncIndex {
	return c.ctrIDIndex
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
func New(ctx context.Context, configIface libconfig.Iface) (*ContainerServer, error) {
	if configIface == nil {
		return nil, errors.New("provided config is nil")
	}
	store, err := configIface.GetStore()
	if err != nil {
		return nil, err
	}
	config := configIface.GetData()

	if config == nil {
		return nil, errors.New("cannot create container server: interface is nil")
	}

	if config.InternalRepair && ShutdownWasUnclean(config) {
		checkOptions := cstorage.CheckEverything()
		report, err := store.Check(checkOptions)
		if err != nil {
			err = HandleUncleanShutdown(config, store)
			if err != nil {
				return nil, err
			}
		}
		options := cstorage.RepairOptions{
			RemoveContainers: true,
		}
		if errs := store.Repair(report, &options); len(errs) > 0 {
			err = HandleUncleanShutdown(config, store)
			if err != nil {
				return nil, err
			}
		}
	}

	imageService, err := storage.GetImageService(ctx, store, nil, config)
	if err != nil {
		return nil, err
	}

	storageRuntimeService := storage.GetRuntimeService(ctx, imageService, nil)

	runtime, err := oci.New(config)
	if err != nil {
		return nil, err
	}

	newHooks, err := hooks.New(ctx, config.HooksDir, []string{})
	if err != nil {
		return nil, err
	}

	c := &ContainerServer{
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
	}
	c.StatsServer = statsserver.New(ctx, c)
	return c, nil
}

// LoadSandbox loads a sandbox from the disk into the sandbox store
func (c *ContainerServer) LoadSandbox(ctx context.Context, id string) (sb *sandbox.Sandbox, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	config, err := c.store.FromContainerDirectory(id, "config.json")
	if err != nil {
		return nil, err
	}
	var m rspec.Spec
	if err := json.Unmarshal(config, &m); err != nil {
		return nil, fmt.Errorf("error unmarshalling sandbox spec: %w", err)
	}

	name, err := c.ReservePodName(id, m.Annotations[annotations.Name])
	if err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			c.ReleasePodName(name)
		}
	}()

	sb, err = sandbox.FromSpec(id, &m)
	if err != nil {
		return nil, fmt.Errorf("sandbox from spec: %w", err)
	}

	defer func() {
		if retErr != nil {
			if err := sb.RemoveManagedNamespaces(); err != nil {
				log.Warnf(ctx, "Failed to remove namespaces: %v", err)
			}
		}
	}()
	if err := c.AddSandbox(ctx, sb); err != nil {
		return sb, err
	}

	defer func() {
		if retErr != nil {
			if err := c.RemoveSandbox(ctx, sb.ID()); err != nil {
				log.Warnf(ctx, "Could not remove sandbox ID %s: %v", sb.ID(), err)
			}
		}
	}()

	sandboxPath, err := c.store.ContainerRunDirectory(id)
	if err != nil {
		return sb, err
	}

	sandboxDir, err := c.store.ContainerDirectory(id)
	if err != nil {
		return sb, err
	}

	cID := m.Annotations[annotations.ContainerID]

	cname, err := c.ReserveContainerName(cID, m.Annotations[annotations.ContainerName])
	if err != nil {
		return sb, err
	}
	defer func() {
		if retErr != nil {
			c.ReleaseContainerName(ctx, cname)
		}
	}()

	var scontainer *oci.Container

	// We should not take whether the server currently has DropInfraCtr specified, but rather
	// whether the server used to.
	wasSpoofed := false
	if spoofed, ok := m.Annotations[annotations.SpoofedContainer]; ok && spoofed == "true" {
		wasSpoofed = true
	}

	created := time.Unix(0, sb.CreatedAt())
	if !wasSpoofed {
		scontainer, err = oci.NewContainer(m.Annotations[annotations.ContainerID], cname, sandboxPath, m.Annotations[annotations.LogPath], sb.Labels(), m.Annotations, sb.Annotations(), m.Annotations[annotations.Image], nil, nil, "", nil, id, false, false, false, sb.RuntimeHandler(), sandboxDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
		if err != nil {
			return sb, err
		}
	} else {
		scontainer = oci.NewSpoofedContainer(cID, cname, sb.Labels(), id, created, sandboxPath)
	}
	scontainer.SetSpec(&m)
	scontainer.SetMountPoint(m.Annotations[annotations.MountPoint])

	if m.Annotations[annotations.Volumes] != "" {
		containerVolumes := []oci.ContainerVolume{}
		if err = json.Unmarshal([]byte(m.Annotations[annotations.Volumes]), &containerVolumes); err != nil {
			return sb, fmt.Errorf("failed to unmarshal container volumes: %w", err)
		}
		for _, cv := range containerVolumes {
			scontainer.AddVolume(cv)
		}
	}

	if err := sb.SetInfraContainer(scontainer); err != nil {
		return sb, err
	}

	// We should restore the infraContainer to the container state store
	c.AddInfraContainer(ctx, scontainer)

	sb.RestoreStopped()
	// We add an NS only if we can load a permanent one.
	// Otherwise, the sandbox will live in the host namespace.
	namespacesToJoin := []struct {
		rspecNS  rspec.LinuxNamespaceType
		joinFunc func(string) error
	}{
		{rspecNS: rspec.NetworkNamespace, joinFunc: sb.NetNsJoin},
		{rspecNS: rspec.IPCNamespace, joinFunc: sb.IpcNsJoin},
		{rspecNS: rspec.UTSNamespace, joinFunc: sb.UtsNsJoin},
		{rspecNS: rspec.UserNamespace, joinFunc: sb.UserNsJoin},
	}
	for _, namespaceToJoin := range namespacesToJoin {
		path, err := configNsPath(&m, namespaceToJoin.rspecNS)
		if err == nil {
			if nsErr := namespaceToJoin.joinFunc(path); nsErr != nil {
				return sb, nsErr
			}
		}
	}

	if err := scontainer.FromDisk(); err != nil {
		return sb, fmt.Errorf("error reading sandbox state from disk %q: %w", scontainer.ID(), err)
	}

	// We write back the state because it is possible that crio did not have a chance to
	// read the exit file and persist exit code into the state on reboot.
	if err := c.ContainerStateToDisk(ctx, scontainer); err != nil {
		return sb, fmt.Errorf("failed to write container %q state to disk: %w", scontainer.ID(), err)
	}

	sb.SetCreated()
	if err := label.ReserveLabel(m.Process.SelinuxLabel); err != nil {
		return sb, err
	}

	if err := c.ctrIDIndex.Add(scontainer.ID()); err != nil {
		return sb, err
	}
	defer func() {
		if retErr != nil {
			if err1 := c.ctrIDIndex.Delete(scontainer.ID()); err1 != nil {
				log.Warnf(ctx, "Could not delete container ID %s: %v", scontainer.ID(), err1)
			}
		}
	}()
	if err := c.podIDIndex.Add(id); err != nil {
		return sb, err
	}
	return sb, nil
}

var ErrIsNonCrioContainer = errors.New("non CRI-O container")

// LoadContainer loads a container from the disk into the container store
func (c *ContainerServer) LoadContainer(ctx context.Context, id string) (retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	config, err := c.store.FromContainerDirectory(id, "config.json")
	if err != nil {
		return err
	}
	var m rspec.Spec
	if err := json.Unmarshal(config, &m); err != nil {
		return err
	}

	// Do not interact with containers of others
	if manager, ok := m.Annotations[annotations.ContainerManager]; ok && manager != ContainerManagerCRIO {
		return ErrIsNonCrioContainer
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
		if retErr != nil {
			c.ReleaseContainerName(ctx, name)
		}
	}()

	var metadata types.ContainerMetadata
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

	userRequestedImage, ok := m.Annotations[annotations.Image]
	if !ok {
		userRequestedImage = ""
	}

	var imgName *references.RegistryImageReference
	if s, ok := m.Annotations[annotations.ImageName]; ok && s != "" {
		name, err := references.ParseRegistryImageReferenceFromOutOfProcessData(s)
		if err != nil {
			return fmt.Errorf("invalid %s annotation %q: %w", annotations.ImageName, s, err)
		}
		imgName = &name
	}

	var imageID *storage.StorageImageID
	if s, ok := m.Annotations[annotations.ImageRef]; ok {
		id, err := storage.ParseStorageImageIDFromOutOfProcessData(s)
		if err != nil {
			return fmt.Errorf("invalid %s annotation %q: %w", annotations.ImageRef, s, err)
		}
		imageID = &id
	}

	platformRuntimePath, ok := m.Annotations[annotations.PlatformRuntimePath]
	if !ok {
		platformRuntimePath = ""
	}

	kubeAnnotations := make(map[string]string)
	if err := json.Unmarshal([]byte(m.Annotations[annotations.Annotations]), &kubeAnnotations); err != nil {
		return err
	}

	created, err := time.Parse(time.RFC3339Nano, m.Annotations[annotations.Created])
	if err != nil {
		return err
	}

	ctr, err := oci.NewContainer(id, name, containerPath, m.Annotations[annotations.LogPath], labels, m.Annotations, kubeAnnotations, userRequestedImage, imgName, imageID, "", &metadata, sb.ID(), tty, stdin, stdinOnce, sb.RuntimeHandler(), containerDir, created, m.Annotations["org.opencontainers.image.stopSignal"])
	if err != nil {
		return err
	}
	ctr.SetSpec(&m)
	ctr.SetMountPoint(m.Annotations[annotations.MountPoint])
	spp := m.Annotations[annotations.SeccompProfilePath]
	ctr.SetSeccompProfilePath(spp)

	if err := ctr.FromDisk(); err != nil {
		return fmt.Errorf("error reading container state from disk %q: %w", ctr.ID(), err)
	}

	// We write back the state because it is possible that crio did not have a chance to
	// read the exit file and persist exit code into the state on reboot.
	if err := c.ContainerStateToDisk(ctx, ctr); err != nil {
		return fmt.Errorf("failed to write container state to disk %q: %w", ctr.ID(), err)
	}
	ctr.SetCreated()

	ctr.SetRuntimePathForPlatform(platformRuntimePath)

	c.AddContainer(ctx, ctr)

	return c.ctrIDIndex.Add(id)
}

func isTrue(annotaton string) bool {
	return annotaton == "true"
}

// ContainerStateToDisk writes the container's state information to a JSON file
// on disk
func (c *ContainerServer) ContainerStateToDisk(ctx context.Context, ctr *oci.Container) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if err := c.Runtime().UpdateContainerStatus(ctx, ctr); err != nil {
		log.Warnf(ctx, "Error updating the container status %q: %v", ctr.ID(), err)
	}

	jsonSource, err := ioutils.NewAtomicFileWriter(ctr.StatePath(), 0o644)
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
		err = fmt.Errorf("error reserving ctr name %s for id %s: %w", name, id, err)
		logrus.Warn(err)
		return "", err
	}
	return name, nil
}

// ContainerIDForName gets the container ID given the container name from the ID Index
func (c *ContainerServer) ContainerIDForName(name string) (string, error) {
	return c.ctrNameIndex.Get(name)
}

// ReleaseContainerName releases a container name from the index so that it can
// be used by other containers
func (c *ContainerServer) ReleaseContainerName(ctx context.Context, name string) {
	_, span := log.StartSpan(ctx)
	defer span.End()
	c.ctrNameIndex.Release(name)
}

// ReservePodName holds a name for a pod that is being created
func (c *ContainerServer) ReservePodName(id, name string) (string, error) {
	if err := c.podNameIndex.Reserve(name, id); err != nil {
		err = fmt.Errorf("error reserving pod name %s for id %s: %w", name, id, err)
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

// PodIDForName gets the pod ID given the pod name from the ID Index
func (c *ContainerServer) PodIDForName(name string) (string, error) {
	return c.podNameIndex.Get(name)
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
	if err != nil && !errors.Is(err, cstorage.ErrLayerUsedByContainer) {
		return err
	}
	c.StatsServer.Shutdown()
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
func (c *ContainerServer) AddContainer(ctx context.Context, ctr *oci.Container) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	newSandbox := c.state.sandboxes.Get(ctr.Sandbox())
	if newSandbox == nil {
		return
	}
	newSandbox.AddContainer(ctx, ctr)
	c.state.containers.Add(ctr.ID(), ctr)
}

// AddInfraContainer adds a container to the container state store
func (c *ContainerServer) AddInfraContainer(ctx context.Context, ctr *oci.Container) {
	c.state.infraContainers.Add(ctr.ID(), ctr)
}

// GetContainer returns a container by its ID
func (c *ContainerServer) GetContainer(ctx context.Context, id string) *oci.Container {
	return c.state.containers.Get(id)
}

// GetInfraContainer returns a container by its ID
func (c *ContainerServer) GetInfraContainer(ctx context.Context, id string) *oci.Container {
	_, span := log.StartSpan(ctx)
	defer span.End()
	return c.state.infraContainers.Get(id)
}

// HasContainer checks if a container exists in the state
func (c *ContainerServer) HasContainer(id string) bool {
	return c.state.containers.Get(id) != nil
}

// RemoveContainer removes a container from the container state store
func (c *ContainerServer) RemoveContainer(ctx context.Context, ctr *oci.Container) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	sbID := ctr.Sandbox()
	sb := c.state.sandboxes.Get(sbID)
	if sb == nil {
		return
	}
	sb.RemoveContainer(ctx, ctr)
	c.RemoveStatsForContainer(ctr)
	if err := ctr.RemoveManagedPIDNamespace(); err != nil {
		log.Errorf(ctx, "Failed to remove container %s PID namespace: %v", ctr.ID(), err)
	}
	c.state.containers.Delete(ctr.ID())
}

// RemoveInfraContainer removes a container from the container state store
func (c *ContainerServer) RemoveInfraContainer(ctx context.Context, ctr *oci.Container) {
	_, span := log.StartSpan(ctx)
	defer span.End()
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
				break
			}
		}
	}
	return filteredContainers, nil
}

// AddSandbox adds a sandbox to the sandbox state store
func (c *ContainerServer) AddSandbox(ctx context.Context, sb *sandbox.Sandbox) error {
	_, span := log.StartSpan(ctx)
	defer span.End()
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
func (c *ContainerServer) RemoveSandbox(ctx context.Context, id string) error {
	_, span := log.StartSpan(ctx)
	defer span.End()
	sb := c.state.sandboxes.Get(id)
	if sb == nil {
		return nil
	}

	c.stateLock.Lock()
	defer c.stateLock.Unlock()
	if err := c.removeSandboxPlatform(sb); err != nil {
		return err
	}

	c.RemoveStatsForSandbox(sb)
	c.state.sandboxes.Delete(id)
	return nil
}

// ListSandboxes lists all sandboxes in the state store
func (c *ContainerServer) ListSandboxes() []*sandbox.Sandbox {
	return c.state.sandboxes.List()
}

func (c *ContainerServer) UpdateContainerLinuxResources(ctr *oci.Container, resources *rspec.LinuxResources) {
	updatedSpec := ctr.Spec()
	if updatedSpec.Linux == nil {
		updatedSpec.Linux = &rspec.Linux{}
	}

	if updatedSpec.Linux.Resources == nil {
		updatedSpec.Linux.Resources = &rspec.LinuxResources{}
	}

	if updatedSpec.Linux.Resources.CPU == nil {
		updatedSpec.Linux.Resources.CPU = &rspec.LinuxCPU{}
	}

	if resources.CPU.Shares != nil {
		updatedSpec.Linux.Resources.CPU.Shares = resources.CPU.Shares
	}

	if resources.CPU.Quota != nil {
		updatedSpec.Linux.Resources.CPU.Quota = resources.CPU.Quota
	}

	if resources.CPU.Period != nil {
		updatedSpec.Linux.Resources.CPU.Period = resources.CPU.Period
	}

	if resources.CPU.Cpus != "" {
		updatedSpec.Linux.Resources.CPU.Cpus = resources.CPU.Cpus
	}

	if resources.CPU.Mems != "" {
		updatedSpec.Linux.Resources.CPU.Mems = resources.CPU.Mems
	}

	if updatedSpec.Linux.Resources.Memory == nil {
		updatedSpec.Linux.Resources.Memory = &rspec.LinuxMemory{}
	}

	if resources.Memory.Limit != nil {
		updatedSpec.Linux.Resources.Memory.Limit = resources.Memory.Limit
	}

	if resources.Memory.Swap != nil {
		updatedSpec.Linux.Resources.Memory.Swap = resources.Memory.Swap
	}

	ctr.SetSpec(&updatedSpec)

	c.state.containers.Add(ctr.ID(), ctr)
}

func ShutdownWasUnclean(config *libconfig.Config) bool {
	// CleanShutdownFile not configured, skip
	if config.CleanShutdownFile == "" {
		return false
	}
	// CleanShutdownFile isn't supported, skip
	if _, err := os.Stat(config.CleanShutdownSupportedFileName()); err != nil {
		return false
	}
	// CleanShutdownFile is present, indicating clean shutdown
	if _, err := os.Stat(config.CleanShutdownFile); err == nil {
		return false
	}
	return true
}

func HandleUncleanShutdown(config *libconfig.Config, store cstorage.Store) error {
	logrus.Infof("File %s not found. Wiping storage directory %s because of suspected dirty shutdown", config.CleanShutdownFile, store.GraphRoot())
	// If we do not do this, we may leak other resources that are not directly in the graphroot.
	// Erroring here should not be fatal though, it's a best effort cleanup
	if err := store.Wipe(); err != nil {
		logrus.Infof("Failed to wipe storage cleanly: %v", err)
	}
	// unmount storage or else we will fail with EBUSY
	if _, err := store.Shutdown(false); err != nil {
		return fmt.Errorf("failed to shutdown storage before wiping: %w", err)
	}
	// totally remove storage, whatever is left (possibly orphaned layers)
	if err := os.RemoveAll(store.GraphRoot()); err != nil {
		return fmt.Errorf("failed to remove storage directory: %w", err)
	}
	return nil
}
