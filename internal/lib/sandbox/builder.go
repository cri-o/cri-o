package sandbox

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/stringid"
	"github.com/opencontainers/runtime-tools/generate"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

// Builder is the interface for managing pod sandboxes.
type Builder interface {
	// SetConfig sets the sandbox configuration and validates it
	SetConfig(*types.PodSandboxConfig) error

	// SetNameAndID sets the sandbox name and ID
	SetNameAndID() error

	// Config returns the sandbox configuration
	Config() *types.PodSandboxConfig

	// ID returns the id of the pod sandbox
	ID() string

	// Name returns the id of the pod sandbox
	Name() string

	// InitInfraContainer initializes the sandbox's infra container
	InitInfraContainer(*libconfig.Config, *storage.ContainerInfo, *idtools.IDMappings) error

	// Spec returns the infra container's generator.
	// Must be called after InitInfraContainer.
	Spec() *generate.Generator

	// ResolvPath returns the sandbox's resolvPath.
	ResolvPath() string

	// SetDNSConfig sets the dns configs.
	SetDNSConfig(*types.DNSConfig)

	// SetCriSandbox sets and creates CriSandbox.
	SetCriSandbox(string, time.Time, map[string]string, map[string]string, *types.PodSandboxMetadata)

	// GetSandbox gets the sandbox and resets the config and sandbox.
	GetSandbox() *Sandbox

	// SetNamespace sets the namespace.
	SetNamespace(string)

	// SetName sets the name for the sandbox
	SetName(string)

	// SetKubeName sets the kubename.
	SetKubeName(string)

	// SetLogDir sets the logDir of the sandbox
	SetLogDir(string)

	// SetContainers sets the containers.
	SetContainers(oci.ContainerStorer)

	// SetProcessLabel sets the processLabel.
	SetProcessLabel(string)

	// SetMountLabel sets the mountLabel.
	SetMountLabel(string)

	// SetShmPath sets the shim path.
	SetShmPath(string)

	// SetCgroupParent sets the cgroup parent.
	SetCgroupParent(string)

	// SetPrivileged sets the privileged.
	SetPrivileged(bool)

	// SetRuntimeHandler sets the runtime handler.
	SetRuntimeHandler(string)

	// SetResolvPath sets the resolv.conf path.
	SetResolvPath(string)

	// SetHostname sets the hostname.
	SetHostname(string)

	// SetPortMappings sets the port mappings.
	SetPortMappings([]*hostport.PortMapping)

	// SetHostNetwork sets the host network.
	SetHostNetwork(bool)

	// SetUsernsMode sets the user namespace mode.
	SetUsernsMode(string)

	// SetPodLinuxOverhead sets the PodLinuxOverhead.
	SetPodLinuxOverhead(*types.LinuxContainerResources)

	// SetPodLinuxResources sets the PodLinuxResources.
	SetPodLinuxResources(*types.LinuxContainerResources)

	// SetHostnamePath sets the hostname path.
	SetHostnamePath(string)

	// SetNamespaceOptions sets the namespace options.
	SetNamespaceOptions(*types.NamespaceOption)

	// SetSeccompProfilePath sets the selinux comp profile path.
	SetSeccompProfilePath(string)

	// SetID sets the Id inside the criSandbox object or creates it.
	SetID(string)
}

// builder is the hidden default type behind the Sandbox interface.
type builder struct {
	config *types.PodSandboxConfig
	infra  container.Container
	sb     *Sandbox
}

// NewBuilder creates a new, empty Sandbox instance.
func NewBuilder() Builder {
	return &builder{
		config: nil,
		sb:     new(Sandbox),
	}
}

func (b *builder) GetSandbox() *Sandbox {
	sb := b.sb
	b.config = nil
	b.sb = nil
	return sb
}

// SetConfig sets the sandbox configuration and validates it.
func (b *builder) SetConfig(config *types.PodSandboxConfig) error {
	if b.config != nil {
		return errors.New("config already set")
	}

	if config == nil {
		return errors.New("config is nil")
	}

	if config.Metadata == nil {
		return errors.New("metadata is nil")
	}

	if config.Metadata.Name == "" {
		return errors.New("metadata.Name should not be empty")
	}

	if config.Linux == nil {
		config.Linux = &types.LinuxPodSandboxConfig{}
	}

	if config.Linux.SecurityContext == nil {
		config.Linux.SecurityContext = &types.LinuxSandboxSecurityContext{
			NamespaceOptions: &types.NamespaceOption{},
			SelinuxOptions:   &types.SELinuxOption{},
			RunAsUser:        &types.Int64Value{},
			RunAsGroup:       &types.Int64Value{},
			Seccomp:          &types.SecurityProfile{},
			Apparmor:         &types.SecurityProfile{},
		}
	}

	b.config = config
	return nil
}

// SetNameAndID sets the sandbox name and ID.
func (b *builder) SetNameAndID() error {
	if b.config == nil {
		return errors.New("config is nil")
	}

	if b.config.Metadata.Namespace == "" {
		return errors.New("cannot generate pod name without namespace")
	}

	if b.config.Metadata.Name == "" {
		return errors.New("cannot generate pod name without name in metadata")
	}

	if b.config.Metadata.Uid == "" {
		return errors.New("cannot generate pod name without uid in metadata")
	}

	id := stringid.GenerateNonCryptoID()
	b.SetID(id)
	b.sb.name = strings.Join([]string{
		"k8s",
		b.config.Metadata.Name,
		b.config.Metadata.Namespace,
		b.config.Metadata.Uid,
		strconv.FormatUint(uint64(b.config.Metadata.Attempt), 10),
	}, "_")

	return nil
}

// Config returns the sandbox configuration.
func (b *builder) Config() *types.PodSandboxConfig {
	return b.config
}

// ID returns the id of the pod sandbox.
func (b *builder) ID() string {
	if b.sb == nil || b.sb.criSandbox == nil {
		return ""
	}
	return b.sb.criSandbox.Id
}

// Name returns the id of the pod sandbox.
func (b *builder) Name() string {
	return b.sb.name
}

func (b *builder) ResolvPath() string {
	return b.sb.resolvPath
}

// SetDNSConfig sets the DNSConfig.
func (b *builder) SetDNSConfig(dnsConfig *types.DNSConfig) {
	b.config.DnsConfig = dnsConfig
}

func (b *builder) SetCriSandbox(id string, createdAt time.Time, labels, annotations map[string]string, metadata *types.PodSandboxMetadata) {
	if b.sb.criSandbox != nil {
		b.sb.criSandbox.CreatedAt = createdAt.UnixNano()
		b.sb.criSandbox.Annotations = annotations
		b.sb.criSandbox.Labels = labels
		b.sb.criSandbox.Metadata = metadata
	}
	b.sb.criSandbox = &types.PodSandbox{
		Id:          id,
		CreatedAt:   createdAt.UnixNano(),
		Labels:      labels,
		Annotations: annotations,
		Metadata:    metadata,
	}
}

// SetNamespace sets the namespace for the sidecar container.
func (b *builder) SetNamespace(namespace string) {
	b.sb.namespace = namespace
}

// SetName sets the name for the sidecar container.
func (b *builder) SetName(name string) {
	b.sb.name = name
}

// SetKubeName sets the Kubernetes name for the sidecar container.
func (b *builder) SetKubeName(kubeName string) {
	b.sb.kubeName = kubeName
}

// SetLogDir sets the log directory for the sidecar container.
func (b *builder) SetLogDir(logDir string) {
	b.sb.logDir = logDir
}

// SetContainers sets the container configuration for the sidecar (using a pointer to avoid unnecessary copies).
func (b *builder) SetContainers(containers oci.ContainerStorer) {
	b.sb.containers = containers
}

// SetProcessLabel sets the process label for the sidecar container.
func (b *builder) SetProcessLabel(processLabel string) {
	b.sb.processLabel = processLabel
}

// SetMountLabel sets the mount label for the sidecar container.
func (b *builder) SetMountLabel(mountLabel string) {
	b.sb.mountLabel = mountLabel
}

// SetShmPath sets the shared memory path for the sidecar container.
func (b *builder) SetShmPath(shmPath string) {
	b.sb.shmPath = shmPath
}

// SetCgroupParent sets the cgroup parent for the sidecar container.
func (b *builder) SetCgroupParent(cgroupParent string) {
	b.sb.cgroupParent = cgroupParent
}

// SetPrivileged sets the privileged flag for the sidecar container.
func (b *builder) SetPrivileged(privileged bool) {
	b.sb.privileged = privileged
}

// SetRuntimeHandler sets the runtime handler for the sidecar container.
func (b *builder) SetRuntimeHandler(runtimeHandler string) {
	b.sb.runtimeHandler = runtimeHandler
}

// SetResolvPath sets the resolv path for the sidecar container.
func (b *builder) SetResolvPath(resolvPath string) {
	b.sb.resolvPath = resolvPath
}

// SetHostname sets the hostname for the sidecar container.
func (b *builder) SetHostname(hostname string) {
	b.sb.hostname = hostname
}

// SetPortMappings sets the port mappings for the sidecar container.
func (b *builder) SetPortMappings(portMappings []*hostport.PortMapping) {
	b.sb.portMappings = portMappings
}

// SetHostNetwork sets the host network flag for the sidecar container.
func (b *builder) SetHostNetwork(hostNetwork bool) {
	b.sb.hostNetwork = hostNetwork
}

// SetUsernsMode sets the user namespace mode for the sidecar container.
func (b *builder) SetUsernsMode(usernsMode string) {
	b.sb.usernsMode = usernsMode
}

// SetPodLinuxOverhead sets the pod Linux overhead for the sidecar container.
func (b *builder) SetPodLinuxOverhead(podLinuxOverhead *types.LinuxContainerResources) {
	b.sb.podLinuxOverhead = podLinuxOverhead
}

// SetPodLinuxResources sets the pod Linux resources for the sidecar container.
func (b *builder) SetPodLinuxResources(podLinuxResources *types.LinuxContainerResources) {
	b.sb.podLinuxResources = podLinuxResources
}

// SetHostnamePath adds the hostname path to the sandbox.
func (b *builder) SetHostnamePath(hostnamePath string) {
	b.sb.hostnamePath = hostnamePath
}

// SetNamespaceOptions sets whether the pod is running using host network.
func (b *builder) SetNamespaceOptions(nsOpts *types.NamespaceOption) {
	b.sb.nsOpts = nsOpts
}

// SetSeccompProfilePath sets the seccomp profile path.
func (b *builder) SetSeccompProfilePath(profilePath string) {
	b.sb.seccompProfilePath = profilePath
}

func (b *builder) SetID(id string) {
	if b.sb.criSandbox != nil {
		b.sb.criSandbox.Id = id
	} else {
		b.sb.criSandbox = &types.PodSandbox{
			Id: id,
		}
	}
}
