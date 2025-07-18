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
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

// Builder is the interface for managing pod sandboxes.
type Builder interface {
	// SetConfig sets the sandbox configuration and validates it
	SetConfig(*types.PodSandboxConfig) error

	// GenerateNameAndID sets the sandbox name and ID
	GenerateNameAndID() error

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

	// SetCRISandbox sets and creates CRI sandbox with the given parameters.
	SetCRISandbox(string, map[string]string, map[string]string, *types.PodSandboxMetadata) error

	// GetSandbox gets the sandbox and deletes the config and sandbox.
	GetSandbox() (*Sandbox, error)

	// SetNamespace sets the namespace.
	SetNamespace(string)

	// SetName sets the name for the sandbox
	SetName(string)

	// SetKubeName sets the kubename.
	SetKubeName(string)

	// SetLogDir sets the logDir of the sandbox
	SetLogDir(string)

	// SetContainers sets the containers.
	SetContainers(memorystore.Storer[*oci.Container])

	// SetProcessLabel sets the processLabel.
	SetProcessLabel(string)

	// SetMountLabel sets the mountLabel.
	SetMountLabel(string)

	// SetShmPath sets the shared memory path.
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

	// SetSeccompProfilePath sets the seccomp profile path.
	SetSeccompProfilePath(string)

	// SetID sets the Id inside the criSandbox object or creates it.
	SetID(string)

	// SetCreatedAt sets the created at time.
	SetCreatedAt(createdAt time.Time)

	// Validate validates the sandbox.
	Validate() error
}

type sandboxValidations uint64

const (
	validationSetID sandboxValidations = 1 << iota
	validationSetCRISandbox
	validationSetCreatedAt
	validationSetName
	validationSetLogDir
	validationSetShmPath
	validationSetNamespace
	validationSetKubeName
	validationSetProcessLabel
	validationSetMountLabel
	validationSetCgroupParent
	validationSetPrivileged
	validationSetRuntimeHandler
	validationSetResolvPath
	validationSetHostname
	validationSetPortMappings
	validationSetHostNetwork
	validationSetUsernsMode
	validationSetPodLinuxOverhead
	validationSetPodLinuxResources
)

// setValidation sets the given validation flag.
func (v *sandboxValidations) setValidation(flag sandboxValidations) {
	*v |= flag
}

// isValidationSet returns true if the given validation flag is set.
func (v *sandboxValidations) isValidationSet(flag sandboxValidations) bool {
	return *v&flag != 0
}

// sandboxBuilder is the hidden default type behind the Sandbox interface.
type sandboxBuilder struct {
	config      *types.PodSandboxConfig
	infra       container.Container
	sandboxRef  *Sandbox
	validations sandboxValidations
}

// NewBuilder creates a new, empty Sandbox instance.
func NewBuilder() Builder {
	return &sandboxBuilder{
		config:      nil,
		sandboxRef:  new(Sandbox),
		validations: 0,
	}
}

// GetSandbox gets the sandbox and deletes the config and sandbox.
// TODO: Add validations before returning the sandbox.
func (b *sandboxBuilder) GetSandbox() (*Sandbox, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}

	sandboxRef := b.sandboxRef
	b.config = nil
	b.sandboxRef = nil
	b.validations = 0

	return sandboxRef, nil
}

// allValidations represents all required validation flags combined.
const allValidations sandboxValidations = validationSetCRISandbox |
	validationSetCreatedAt |
	validationSetID |
	validationSetName |
	validationSetLogDir |
	validationSetShmPath |
	validationSetNamespace |
	validationSetKubeName |
	validationSetProcessLabel |
	validationSetMountLabel |
	validationSetCgroupParent |
	validationSetPrivileged |
	validationSetRuntimeHandler |
	validationSetResolvPath |
	validationSetHostname |
	validationSetPortMappings |
	validationSetHostNetwork |
	validationSetUsernsMode |
	validationSetPodLinuxOverhead |
	validationSetPodLinuxResources

// If not all validations are set, check which ones are missing.
var requiredValidations = map[string]sandboxValidations{
	"cri-o sandbox":                validationSetCRISandbox,
	"createdAt time":               validationSetCreatedAt,
	"sandbox id":                   validationSetID,
	"sandbox name":                 validationSetName,
	"sandbox logDir":               validationSetLogDir,
	"sandbox shmPath":              validationSetShmPath,
	"sandbox setNamespace":         validationSetNamespace,
	"sandbox setKubeName":          validationSetKubeName,
	"sandbox setProcessLabel":      validationSetProcessLabel,
	"sandbox setMountLabel":        validationSetMountLabel,
	"sandbox setCgroupParent":      validationSetCgroupParent,
	"sandbox setPrivileged":        validationSetPrivileged,
	"sandbox setRuntimeHandler":    validationSetRuntimeHandler,
	"sandbox setResolvPath":        validationSetResolvPath,
	"sandbox setHostname":          validationSetHostname,
	"sandbox setPortMappings":      validationSetPortMappings,
	"sandbox setHostNetwork":       validationSetHostNetwork,
	"sandbox setUsernsMode":        validationSetUsernsMode,
	"sandbox setPodLinuxOverhead":  validationSetPodLinuxOverhead,
	"sandbox setPodLinuxResources": validationSetPodLinuxResources,
}

// Validate validates the sandbox.
func (b *sandboxBuilder) Validate() error {
	// Quick check if all validations are set
	if b.validations == allValidations {
		return nil
	}

	for field, flag := range requiredValidations {
		if !b.validations.isValidationSet(flag) {
			return errors.New(field + " not set")
		}
	}

	return nil
}

// SetConfig sets the sandbox configuration and validates it.
func (b *sandboxBuilder) SetConfig(config *types.PodSandboxConfig) error {
	if b.config != nil {
		return errors.New("config already set")
	}

	if config == nil {
		return errors.New("config is nil")
	}

	if config.GetMetadata() == nil {
		return errors.New("metadata is nil")
	}

	if config.GetMetadata().GetName() == "" {
		return errors.New("metadata.Name should not be empty")
	}

	if config.GetLinux() == nil {
		config.Linux = &types.LinuxPodSandboxConfig{}
	}

	if config.GetLinux().GetSecurityContext() == nil {
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

// GenerateNameAndID sets the sandbox name and ID.
func (b *sandboxBuilder) GenerateNameAndID() error {
	b.validations.setValidation(validationSetName)

	if b.config == nil {
		return errors.New("config is nil")
	}

	if b.config.GetMetadata().GetNamespace() == "" {
		return errors.New("cannot generate pod name without namespace")
	}

	if b.config.GetMetadata().GetName() == "" {
		return errors.New("cannot generate pod name without name in metadata")
	}

	if b.config.GetMetadata().GetUid() == "" {
		return errors.New("cannot generate pod name without uid in metadata")
	}

	id := stringid.GenerateNonCryptoID()
	b.SetID(id)
	name := strings.Join([]string{
		"k8s",
		b.config.GetMetadata().GetName(),
		b.config.GetMetadata().GetNamespace(),
		b.config.GetMetadata().GetUid(),
		strconv.FormatUint(uint64(b.config.GetMetadata().GetAttempt()), 10),
	}, "_")
	b.SetName(name)

	return nil
}

// Config returns the sandbox configuration.
func (b *sandboxBuilder) Config() *types.PodSandboxConfig {
	return b.config
}

// ID returns the id of the pod sandbox.
func (b *sandboxBuilder) ID() string {
	if b.sandboxRef == nil || b.sandboxRef.criSandbox == nil {
		return ""
	}

	return b.sandboxRef.criSandbox.GetId()
}

// Name returns the name of the pod sandbox.
func (b *sandboxBuilder) Name() string {
	return b.sandboxRef.name
}

func (b *sandboxBuilder) ResolvPath() string {
	return b.sandboxRef.resolvPath
}

// SetDNSConfig sets the DNSConfig.
func (b *sandboxBuilder) SetDNSConfig(dnsConfig *types.DNSConfig) {
	b.config.DnsConfig = dnsConfig
}

// SetCRISandbox sets the CRISandbox.
// TODO: Consider breaking this to separate Create and Update functions.
func (b *sandboxBuilder) SetCRISandbox(id string, labels, annotations map[string]string, metadata *types.PodSandboxMetadata) error {
	b.validations.setValidation(validationSetCRISandbox)

	if b.sandboxRef.createdAt.IsZero() {
		return errors.New("createdAt time is Zero")
	}

	if b.sandboxRef.criSandbox != nil {
		b.sandboxRef.criSandbox.CreatedAt = b.sandboxRef.createdAt.UnixNano()
		b.sandboxRef.criSandbox.Annotations = annotations
		b.sandboxRef.criSandbox.Labels = labels
		b.sandboxRef.criSandbox.Metadata = metadata

		return nil
	}

	b.sandboxRef.criSandbox = &types.PodSandbox{
		Id:          id,
		CreatedAt:   b.sandboxRef.createdAt.UnixNano(),
		Labels:      labels,
		Annotations: annotations,
		Metadata:    metadata,
	}

	return nil
}

// SetNamespace sets the namespace for the sidecar container.
func (b *sandboxBuilder) SetNamespace(namespace string) {
	b.validations.setValidation(validationSetNamespace)
	b.sandboxRef.namespace = namespace
}

// SetName sets the name for the sidecar container.
func (b *sandboxBuilder) SetName(name string) {
	b.validations.setValidation(validationSetName)
	b.sandboxRef.name = name
}

// SetKubeName sets the Kubernetes name for the sidecar container.
func (b *sandboxBuilder) SetKubeName(kubeName string) {
	b.validations.setValidation(validationSetKubeName)
	b.sandboxRef.kubeName = kubeName
}

// SetLogDir sets the log directory for the sidecar container.
func (b *sandboxBuilder) SetLogDir(logDir string) {
	b.validations.setValidation(validationSetLogDir)
	b.sandboxRef.logDir = logDir
}

// SetContainers sets the container configuration for the sidecar (using a pointer to avoid unnecessary copies).
func (b *sandboxBuilder) SetContainers(containers memorystore.Storer[*oci.Container]) {
	b.sandboxRef.containers = containers
}

// SetProcessLabel sets the process label for the sidecar container.
func (b *sandboxBuilder) SetProcessLabel(processLabel string) {
	b.validations.setValidation(validationSetProcessLabel)
	b.sandboxRef.processLabel = processLabel
}

// SetMountLabel sets the mount label for the sidecar container.
func (b *sandboxBuilder) SetMountLabel(mountLabel string) {
	b.validations.setValidation(validationSetMountLabel)
	b.sandboxRef.mountLabel = mountLabel
}

// SetShmPath sets the shared memory path for the sidecar container.
func (b *sandboxBuilder) SetShmPath(shmPath string) {
	b.validations.setValidation(validationSetShmPath)
	b.sandboxRef.shmPath = shmPath
}

// SetCgroupParent sets the cgroup parent for the sidecar container.
func (b *sandboxBuilder) SetCgroupParent(cgroupParent string) {
	b.validations.setValidation(validationSetCgroupParent)
	b.sandboxRef.cgroupParent = cgroupParent
}

// SetPrivileged sets the privileged flag for the sidecar container.
func (b *sandboxBuilder) SetPrivileged(privileged bool) {
	b.validations.setValidation(validationSetPrivileged)
	b.sandboxRef.privileged = privileged
}

// SetRuntimeHandler sets the runtime handler for the sidecar container.
func (b *sandboxBuilder) SetRuntimeHandler(runtimeHandler string) {
	b.validations.setValidation(validationSetRuntimeHandler)
	b.sandboxRef.runtimeHandler = runtimeHandler
}

// SetResolvPath sets the resolv path for the sidecar container.
func (b *sandboxBuilder) SetResolvPath(resolvPath string) {
	b.validations.setValidation(validationSetResolvPath)
	b.sandboxRef.resolvPath = resolvPath
}

// SetHostname sets the hostname for the sidecar container.
func (b *sandboxBuilder) SetHostname(hostname string) {
	b.validations.setValidation(validationSetHostname)
	b.sandboxRef.hostname = hostname
}

// SetPortMappings sets the port mappings for the sidecar container.
func (b *sandboxBuilder) SetPortMappings(portMappings []*hostport.PortMapping) {
	b.validations.setValidation(validationSetPortMappings)
	b.sandboxRef.portMappings = portMappings
}

// SetHostNetwork sets the host network flag for the sidecar container.
func (b *sandboxBuilder) SetHostNetwork(hostNetwork bool) {
	b.validations.setValidation(validationSetHostNetwork)
	b.sandboxRef.hostNetwork = hostNetwork
}

// SetUsernsMode sets the user namespace mode for the sidecar container.
func (b *sandboxBuilder) SetUsernsMode(usernsMode string) {
	b.validations.setValidation(validationSetUsernsMode)
	b.sandboxRef.usernsMode = usernsMode
}

// SetPodLinuxOverhead sets the pod Linux overhead for the sidecar container.
func (b *sandboxBuilder) SetPodLinuxOverhead(podLinuxOverhead *types.LinuxContainerResources) {
	b.validations.setValidation(validationSetPodLinuxOverhead)
	b.sandboxRef.podLinuxOverhead = podLinuxOverhead
}

// SetPodLinuxResources sets the pod Linux resources for the sidecar container.
func (b *sandboxBuilder) SetPodLinuxResources(podLinuxResources *types.LinuxContainerResources) {
	b.validations.setValidation(validationSetPodLinuxResources)
	b.sandboxRef.podLinuxResources = podLinuxResources
}

// SetHostnamePath adds the hostname path to the sandbox.
func (b *sandboxBuilder) SetHostnamePath(hostnamePath string) {
	b.sandboxRef.hostnamePath = hostnamePath
}

// SetNamespaceOptions sets whether the pod is running using host network.
func (b *sandboxBuilder) SetNamespaceOptions(nsOpts *types.NamespaceOption) {
	b.sandboxRef.nsOpts = nsOpts
}

// SetSeccompProfilePath sets the seccomp profile path.
func (b *sandboxBuilder) SetSeccompProfilePath(profilePath string) {
	b.sandboxRef.seccompProfilePath = profilePath
}

// SetCreatedAt sets the created at time.
func (b *sandboxBuilder) SetCreatedAt(createdAt time.Time) {
	b.validations.setValidation(validationSetCreatedAt)
	b.sandboxRef.createdAt = createdAt
}

func (b *sandboxBuilder) SetID(id string) {
	b.validations.setValidation(validationSetID)

	if b.sandboxRef.criSandbox != nil {
		b.sandboxRef.criSandbox.Id = id
	} else {
		b.sandboxRef.criSandbox = &types.PodSandbox{
			Id: id,
		}
	}
}
