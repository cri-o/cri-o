package builder

import (
	"errors"
	"fmt"
	"github.com/containers/storage/pkg/idtools"
	"github.com/containers/storage/pkg/stringid"
	"github.com/cri-o/cri-o/internal/factory/container"
	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/sys/unix"
	"io"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"os"
	"strconv"
	"strings"
	"time"
)

type sandboxBuilder struct {
	sandbox *sandbox.Sandbox
	infra   container.Container
	config  *types.PodSandboxConfig
}

func (s *sandboxBuilder) SetContainers() {
	s.sandbox.SetContainers()
}

func (s *sandboxBuilder) GetSandbox() *sandbox.Sandbox {
	sb := s.sandbox
	//s.sandbox = nil
	//s.config = nil
	//s.infra = nil
	return sb
}

// SetConfig sets the sandbox configuration and validates it.
func (s *sandboxBuilder) SetConfig(config *types.PodSandboxConfig) error {
	if s.config != nil {
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

	s.config = config
	return nil
}

// SetName sets the sandbox name
func (s *sandboxBuilder) SetName() error {
	if s.config == nil {
		return errors.New("config is nil")
	}

	if s.config.Metadata.Namespace == "" {
		return errors.New("cannot generate pod name without namespace")
	}

	if s.config.Metadata.Name == "" {
		return errors.New("cannot generate pod name without name in metadata")
	}

	if s.config.Metadata.Uid == "" {
		return errors.New("cannot generate pod name without uid in metadata")
	}

	name := strings.Join([]string{
		"k8s",
		s.config.Metadata.Name,
		s.config.Metadata.Namespace,
		s.config.Metadata.Uid,
		strconv.FormatUint(uint64(s.config.Metadata.Attempt), 10),
	}, "_")
	s.sandbox.SetName(name)

	return nil
}

// SetName sets the sandbox name
func (s *sandboxBuilder) SetID() error {
	if s.config == nil {
		return errors.New("config is nil")
	}

	if s.config.Metadata.Namespace == "" {
		return errors.New("cannot generate pod name without namespace")
	}

	if s.config.Metadata.Name == "" {
		return errors.New("cannot generate pod name without name in metadata")
	}

	if s.config.Metadata.Uid == "" {
		return errors.New("cannot generate pod name without uid in metadata")
	}

	s.sandbox.SetId(stringid.GenerateNonCryptoID())

	return nil
}

type SandboxBuilder interface {
	GetSandbox() *sandbox.Sandbox

	SetConfig(*types.PodSandboxConfig) error

	SetName() error

	SetID() error

	SandboxName() (string, error)

	SandboxID() (string, error)

	Config() *types.PodSandboxConfig

	InitInfraContainer(*libconfig.Config, *storage.ContainerInfo, *idtools.IDMappings) error

	Spec() *generate.Generator

	ResolvPath() (string, error)

	SetNamespace(namespace string)
	SetKubeName(kubeName string)
	SetLogDir(logDir string)
	SetProcessLabel(processLabel string)
	SetMountLabel(mountLabel string)
	SetShmPath(shmPath string)
	SetCgroupParent(cgroupParent string)
	SetPrivileged(privileged bool)
	SetRuntimeHandler(runtimeHandler string)
	SetHostname(hostname string)
	SetPortMappings(portMappings []*hostport.PortMapping)
	SetHostNetwork(hostNetwork bool)
	SetUsernsMode(usernsMode string)
	SetPodLinuxOverhead(overhead *types.LinuxContainerResources)
	SetPodLinuxResources(resources *types.LinuxContainerResources)
	SetCriSandbox(createdAt time.Time, labels, annotations map[string]string, metadata *types.PodSandboxMetadata) error
	SetDNSConfig(dnsConfig *types.DNSConfig)
	SetContainers()
}

func NewSandboxBuilder() SandboxBuilder {
	return &sandboxBuilder{
		sandbox: new(sandbox.Sandbox),
	}
}

func (s *sandboxBuilder) Config() *types.PodSandboxConfig {
	return s.config
}

// ID returns the id of the pod sandbox.
func (s *sandboxBuilder) SandboxID() (string, error) {
	if s.sandbox == nil || s.sandbox.CRISandbox() == nil {
		return "", errors.New("cri sandbox not created")
	}
	return s.sandbox.CRISandbox().Id, nil

}

// Name returns the id of the pod sandbox.
func (s *sandboxBuilder) SandboxName() (string, error) {
	if s.sandbox == nil {
		return "", errors.New("sandbox not created")
	}
	return s.sandbox.Name(), nil
}

func (s *sandboxBuilder) ResolvPath() (string, error) {
	if s.sandbox == nil {
		return "", errors.New("sandbox not created")
	}
	return s.sandbox.ResolvPath(), nil
}

func (s *sandboxBuilder) InitInfraContainer(serverConfig *libconfig.Config, podContainer *storage.ContainerInfo, sandboxIDMappings *idtools.IDMappings) error {
	var err error
	s.infra, err = container.New()
	if err != nil {
		return err
	}

	// determine pause command before initializing generator
	// so a failure will not result in a half configured generator
	pauseCommand, err := PauseCommand(serverConfig, podContainer.Config)
	if err != nil {
		return err
	}

	g := s.infra.Spec()
	g.HostSpecific = true
	g.ClearProcessRlimits()

	// setup defaults for the pod sandbox
	g.SetRootReadonly(true)

	// configure default ulimits
	for _, u := range serverConfig.Ulimits() {
		g.AddProcessRlimits(u.Name, u.Hard, u.Soft)
	}
	g.SetProcessArgs(pauseCommand)

	if err := s.createResolvConf(podContainer, sandboxIDMappings); err != nil {
		return fmt.Errorf("create resolv conf: %w", err)
	}

	// Add capabilities from crio.conf if default_capabilities is defined
	if err := s.infra.SpecSetupCapabilities(&types.Capability{}, serverConfig.DefaultCapabilities, serverConfig.AddInheritableCapabilities); err != nil {
		return err
	}

	return nil
}

func (sb *sandboxBuilder) SetNamespace(namespace string) {
	sb.sandbox.SetNamespace(namespace)
}

func (sb *sandboxBuilder) SetKubeName(kubeName string) {
	sb.sandbox.SetKubeName(kubeName)
}

func (sb *sandboxBuilder) SetLogDir(logDir string) {
	sb.sandbox.SetLogDir(logDir)
}

func (sb *sandboxBuilder) SetProcessLabel(processLabel string) {
	sb.sandbox.SetProcessLabel(processLabel)
}

func (sb *sandboxBuilder) SetMountLabel(mountLabel string) {
	sb.sandbox.SetMountLabel(mountLabel)
}

func (sb *sandboxBuilder) SetShmPath(shmPath string) {
	sb.sandbox.SetShmPath(shmPath)
}

func (sb *sandboxBuilder) SetCgroupParent(cgroupParent string) {
	sb.sandbox.SetCgroupParent(cgroupParent)
}

func (sb *sandboxBuilder) SetPrivileged(privileged bool) {
	sb.sandbox.SetPrivileged(privileged)
}

func (sb *sandboxBuilder) SetRuntimeHandler(runtimeHandler string) {
	sb.sandbox.SetRuntimeHandler(runtimeHandler)
}

func (sb *sandboxBuilder) SetHostname(hostname string) {
	sb.sandbox.SetHostname(hostname)
}

func (sb *sandboxBuilder) SetPortMappings(portMappings []*hostport.PortMapping) {
	sb.sandbox.SetPortMappings(portMappings)
}

func (sb *sandboxBuilder) SetHostNetwork(hostNetwork bool) {
	sb.sandbox.SetHostNetwork(hostNetwork)
}

func (sb *sandboxBuilder) SetUsernsMode(usernsMode string) {
	sb.sandbox.SetUsernsMode(usernsMode)
}

func (sb *sandboxBuilder) SetPodLinuxOverhead(overhead *types.LinuxContainerResources) {
	sb.sandbox.SetPodLinuxOverhead(overhead)
}

func (sb *sandboxBuilder) SetPodLinuxResources(resources *types.LinuxContainerResources) {
	sb.sandbox.SetPodLinuxResources(resources)
}

func (sb *sandboxBuilder) SetCriSandbox(createdAt time.Time, labels, annotations map[string]string, metadata *types.PodSandboxMetadata) error {
	return sb.sandbox.SetCriSandbox(createdAt, labels, annotations, metadata)
}

func (s *sandboxBuilder) SetDNSConfig(dnsConfig *types.DNSConfig) {
	s.sandbox.SetDNSConfig(dnsConfig)
}

// Spec can only be called after a successful call to InitInfraContainer.
func (s *sandboxBuilder) Spec() *generate.Generator {
	return s.infra.Spec()
}

// PauseCommand returns the pause command for the provided image configuration.
func PauseCommand(cfg *libconfig.Config, image *v1.Image) ([]string, error) {
	if cfg == nil {
		return nil, errors.New("provided configuration is nil")
	}

	// This has been explicitly set by the user, since the configuration
	// default is `/pause`
	if cfg.PauseCommand != "" {
		return []string{cfg.PauseCommand}, nil
	}
	if image == nil || (len(image.Config.Entrypoint) == 0 && len(image.Config.Cmd) == 0) {
		return nil, fmt.Errorf(
			"unable to run pause image %q: %s",
			cfg.PauseImage,
			"neither Cmd nor Entrypoint specified",
		)
	}
	cmd := []string{}
	cmd = append(cmd, image.Config.Entrypoint...)
	cmd = append(cmd, image.Config.Cmd...)
	return cmd, nil
}

func (s *sandboxBuilder) createResolvConf(podContainer *storage.ContainerInfo, sandboxIDMappings *idtools.IDMappings) (retErr error) {

	resolvPath := podContainer.RunDir + "/resolv.conf"
	// set DNS options
	s.sandbox.SetResolvPath(resolvPath)

	if s.config.DnsConfig == nil {
		// Ref https://github.com/kubernetes/kubernetes/issues/120748#issuecomment-1922220911
		s.config.DnsConfig = &types.DNSConfig{}
	}

	dnsServers := s.config.DnsConfig.Servers
	dnsSearches := s.config.DnsConfig.Searches
	dnsOptions := s.config.DnsConfig.Options
	err := ParseDNSOptions(dnsServers, dnsSearches, dnsOptions, resolvPath)
	defer func() {
		if retErr != nil {
			if err := os.Remove(resolvPath); err != nil {
				retErr = fmt.Errorf("failed to remove resolvPath after failing to create it: %w", retErr)
			}
		}
	}()
	if err != nil {
		return err
	}

	if err := label.Relabel(resolvPath, podContainer.MountLabel, false); err != nil && !errors.Is(err, unix.ENOTSUP) {
		return err
	}
	if sandboxIDMappings != nil {
		rootPair := sandboxIDMappings.RootPair()
		if err := os.Chown(s.sandbox.ResolvPath(), rootPair.UID, rootPair.GID); err != nil {
			return fmt.Errorf("cannot chown %s to %d:%d: %w", resolvPath, rootPair.UID, rootPair.GID, err)
		}
	}
	mnt := spec.Mount{
		Type:        "bind",
		Source:      resolvPath,
		Destination: "/etc/resolv.conf",
		Options:     []string{"ro", "bind", "nodev", "nosuid", "noexec"},
	}
	s.infra.Spec().AddMount(mnt)
	return nil
}

func ParseDNSOptions(servers, searches, options []string, path string) (retErr error) {
	nServers := len(servers)
	nSearches := len(searches)
	nOptions := len(options)
	if nServers == 0 && nSearches == 0 && nOptions == 0 {
		return copyFile("/etc/resolv.conf", path)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if nSearches > 0 {
		_, err = f.WriteString("search " + strings.Join(searches, " ") + "\n")
		if err != nil {
			return err
		}
	}

	if nServers > 0 {
		_, err = f.WriteString("nameserver " + strings.Join(servers, "\nnameserver ") + "\n")
		if err != nil {
			return err
		}
	}

	if nOptions > 0 {
		_, err = f.WriteString("options " + strings.Join(options, " ") + "\n")
		if err != nil {
			return err
		}
	}

	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
