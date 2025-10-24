package container

import (
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/containers/storage/pkg/stringid"
	"github.com/moby/sys/capability"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletTypes "k8s.io/kubelet/pkg/types"

	"github.com/cri-o/cri-o/internal/config/capabilities"
	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/config/device"
	"github.com/cri-o/cri-o/internal/config/node"
	"github.com/cri-o/cri-o/internal/config/nsmgr"
	"github.com/cri-o/cri-o/internal/lib/constants"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils"
)

// Container is the main public container interface.
type Container interface {
	// All set methods are usually called in order of their definition

	// SetConfig sets the configuration to the container and validates it
	SetConfig(*types.ContainerConfig, *types.PodSandboxConfig) error

	// SetNameAndID sets a container name and ID
	// It can either generate a new ID or use an existing ID
	// if specified as parameter (for container restore)
	SetNameAndID(string) error

	// Config returns the container CRI configuration
	Config() *types.ContainerConfig

	// SandboxConfig returns the sandbox CRI configuration
	SandboxConfig() *types.PodSandboxConfig

	// ID returns the container ID
	ID() string

	// Name returns the container name
	Name() string

	// SetPrivileged sets the privileged bool for the container
	SetPrivileged() error

	// Privileged returns whether this container is privileged
	Privileged() bool

	// LogPath returns the log path for the container
	// It takes as input the LogDir of the sandbox, which is used
	// if there is no LogDir configured in the sandbox CRI config
	LogPath(string) (string, error)

	// DisableFips returns whether the container should disable fips mode
	DisableFips() bool

	// UserRequestedImage returns the image specified in the container spec and used to look up the image when creating the container, or an error.
	// The value might evaluate to a different image (or to a different kind of reference!) at any future time.
	UserRequestedImage() (string, error)

	// ReadOnly returns whether the rootfs should be readonly
	// it takes a bool as to whether crio was configured to
	// be readonly, which it defaults to if the container wasn't
	// specifically asked to be read only
	ReadOnly(bool) bool

	// SelinuxLabel returns the container's SelinuxLabel
	// it takes the sandbox's label, which it falls back upon
	SelinuxLabel(string) ([]string, error)

	// SetRestore marks the container as being restored from a checkpoint
	SetRestore(bool)

	// Restore returns if the container is marked as being
	// restored from a checkpoint
	Restore() bool

	// spec functions

	// returns the spec
	Spec() *generate.Generator

	// SpecAddMount adds a mount to the container's spec
	// it takes the rspec mount object
	// if there is already a mount at the path specified, it removes it.
	SpecAddMount(rspec.Mount)

	// SpecAddAnnotations adds annotations to the spec.
	SpecAddAnnotations(ctx context.Context, sb SandboxIFace, containerVolume []oci.ContainerVolume, mountPoint, configStopSignal string, imageResult *storage.ImageResult, isSystemd bool, seccompRef, platformRuntimePath string) error

	// SpecAddDevices adds devices from the server config, and container CRI config
	SpecAddDevices([]device.Device, []device.Device, bool, bool) error

	// SpecInjectCDIDevices injects any requested CDI devices to the container's Spec.
	SpecInjectCDIDevices() error

	// AddUnifiedResourcesFromAnnotations adds the cgroup-v2 resources specified in the io.kubernetes.cri-o.UnifiedCgroup annotation
	AddUnifiedResourcesFromAnnotations(annotationsMap map[string]string) error

	// SpecSetProcessArgs sets the process args in the spec,
	// given the image information and passed-in container config
	SpecSetProcessArgs(imageOCIConfig *v1.Image) error

	// SpecAddNamespaces sets the container's namespaces.
	SpecAddNamespaces(SandboxIFace, *oci.Container, *config.Config) error

	// SpecSetupCapabilities sets up the container's capabilities
	SpecSetupCapabilities(*types.Capability, capabilities.Capabilities, bool) error

	// SpecSetPrivileges sets the container's privileges
	SpecSetPrivileges(ctx context.Context, securityContext *types.LinuxContainerSecurityContext, cfg *config.Config) error

	// SpecSetLinuxContainerResources sets the container resources
	SpecSetLinuxContainerResources(resources *types.LinuxContainerResources, containerMinMemory int64) error

	// PidNamespace returns the pid namespace created by SpecAddNamespaces.
	PidNamespace() nsmgr.Namespace

	// WillRunSystemd checks whether the process args
	// are configured to be run as a systemd instance.
	WillRunSystemd() bool
}

// container is the hidden default type behind the Container interface.
type container struct {
	config     *types.ContainerConfig
	sboxConfig *types.PodSandboxConfig
	id         string
	name       string
	privileged bool
	restore    bool
	spec       generate.Generator
	pidns      nsmgr.Namespace
}

// New creates a new, empty Sandbox instance.
func New() (Container, error) {
	// TODO: use image os
	spec, err := generate.New(runtime.GOOS)
	if err != nil {
		return nil, err
	}

	return &container{
		spec: spec,
	}, nil
}

// SpecAddMount adds a specified mount to the spec
//
//nolint:gocritic // passing the spec mount around here is intentional
func (c *container) SpecAddMount(r rspec.Mount) {
	c.spec.RemoveMount(r.Destination)
	c.spec.AddMount(r)
}

// SpecAddAnnotation adds all annotations to the spec.
func (c *container) SpecAddAnnotations(ctx context.Context, sb SandboxIFace, containerVolumes []oci.ContainerVolume, mountPoint, configStopSignal string, imageResult *storage.ImageResult, isSystemd bool, seccompRef, platformRuntimePath string) (err error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	// Copied from k8s.io/kubernetes/pkg/kubelet/kuberuntime/labels.go
	const podTerminationGracePeriodLabel = "io.kubernetes.pod.terminationGracePeriod"

	kubeAnnotations := c.Config().GetAnnotations()
	created := time.Now()
	labels := c.Config().GetLabels()

	userRequestedImage, err := c.UserRequestedImage()
	if err != nil {
		return err
	}

	logPath, err := c.LogPath(sb.LogDir())
	if err != nil {
		return err
	}

	// Preserve the sandbox annotations. OCI hooks may re-use the sandbox
	// annotation values to apply them to the container later on.
	// The sandbox annotations are already filtered for the allowed
	// annotations, there is no need to check it additionally here.
	for k, v := range sb.Annotations() {
		if k == annotations.OCISeccompBPFHookAnnotation+"/"+c.config.GetMetadata().GetName() {
			// The OCI seccomp BPF hook
			// (https://github.com/containers/oci-seccomp-bpf-hook)
			// uses the annotation io.containers.trace-syscall as indicator
			// to attach a BFP module to the process. The recorded syscalls
			// will be then stored in the output path file (annotation
			// value prefixed with 'of:'). We now add a custom logic to be
			// able to distinguish containers within pods in Kubernetes. If
			// we suffix the container name within the annotation key like
			// this: io.containers.trace-syscall/container
			// Then we will rewrite the key to
			// 'io.containers.trace-syscall' if the metadata name is equal
			// to 'container'. This allows us to trace containers into
			// distinguishable files.
			log.Debugf(ctx,
				"Annotation key for container %q rewritten to %q (value is: %q)",
				c.config.GetMetadata().GetName(), annotations.OCISeccompBPFHookAnnotation, v,
			)

			c.config.Annotations[annotations.OCISeccompBPFHookAnnotation] = v
			c.spec.AddAnnotation(annotations.OCISeccompBPFHookAnnotation, v)
		} else {
			c.spec.AddAnnotation(k, v)
		}
	}

	c.spec.AddAnnotation(annotations.UserRequestedImage, userRequestedImage)

	someNameOfThisImage := ""
	if imageResult.SomeNameOfThisImage != nil {
		someNameOfThisImage = imageResult.SomeNameOfThisImage.StringForOutOfProcessConsumptionOnly()
	}

	c.spec.AddAnnotation(annotations.SomeNameOfTheImage, someNameOfThisImage)
	c.spec.AddAnnotation(annotations.ImageRef, imageResult.ID.IDStringForOutOfProcessConsumptionOnly())
	c.spec.AddAnnotation(annotations.Name, c.Name())
	c.spec.AddAnnotation(annotations.ContainerID, c.ID())
	c.spec.AddAnnotation(annotations.SandboxID, sb.ID())
	c.spec.AddAnnotation(annotations.SandboxName, sb.Name())
	c.spec.AddAnnotation(annotations.ContainerType, annotations.ContainerTypeContainer)
	c.spec.AddAnnotation(annotations.LogPath, logPath)
	c.spec.AddAnnotation(annotations.TTY, strconv.FormatBool(c.Config().GetTty()))
	c.spec.AddAnnotation(annotations.Stdin, strconv.FormatBool(c.Config().GetStdin()))
	c.spec.AddAnnotation(annotations.StdinOnce, strconv.FormatBool(c.Config().GetStdinOnce()))
	c.spec.AddAnnotation(annotations.ResolvPath, sb.ResolvPath())
	c.spec.AddAnnotation(annotations.ContainerManager, constants.ContainerManagerCRIO)
	c.spec.AddAnnotation(annotations.MountPoint, mountPoint)
	c.spec.AddAnnotation(annotations.SeccompProfilePath, seccompRef)
	c.spec.AddAnnotation(annotations.Created, created.Format(time.RFC3339Nano))
	// for retrieving the runtime path for a given platform.
	c.spec.AddAnnotation(annotations.PlatformRuntimePath, platformRuntimePath)

	metadataJSON, err := json.Marshal(c.Config().GetMetadata())
	if err != nil {
		return err
	}

	c.spec.AddAnnotation(annotations.Metadata, string(metadataJSON))

	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return err
	}

	c.spec.AddAnnotation(annotations.Labels, string(labelsJSON))

	volumesJSON, err := json.Marshal(containerVolumes)
	if err != nil {
		return err
	}

	c.spec.AddAnnotation(annotations.Volumes, string(volumesJSON))

	kubeAnnotationsJSON, err := json.Marshal(kubeAnnotations)
	if err != nil {
		return err
	}

	c.spec.AddAnnotation(annotations.Annotations, string(kubeAnnotationsJSON))

	for k, v := range kubeAnnotations {
		c.spec.AddAnnotation(k, v)
	}

	for k, v := range labels {
		c.spec.AddAnnotation(k, v)
	}

	for idx, ip := range sb.IPs() {
		c.spec.AddAnnotation(fmt.Sprintf("%s.%d", annotations.IP, idx), ip)
	}

	if isSystemd {
		if t, ok := kubeAnnotations[podTerminationGracePeriodLabel]; ok {
			// currently only supported by systemd, see
			// https://github.com/opencontainers/runc/pull/2224
			c.spec.AddAnnotation("org.systemd.property.TimeoutStopUSec", "uint64 "+t+"000000") // sec to usec
		}

		c.spec.AddAnnotation("org.systemd.property.DefaultDependencies", "true")
		c.spec.AddAnnotation("org.systemd.property.After", "['crio.service']")
	}

	if configStopSignal != "" {
		// this key is defined in image-spec conversion document at https://github.com/opencontainers/image-spec/pull/492/files#diff-8aafbe2c3690162540381b8cdb157112R57
		c.spec.AddAnnotation(annotations.StopSignalAnnotation, configStopSignal)
	}

	return nil
}

func (c *container) Spec() *generate.Generator {
	return &c.spec
}

// SetConfig sets the configuration to the container and validates it.
func (c *container) SetConfig(cfg *types.ContainerConfig, sboxConfig *types.PodSandboxConfig) error {
	if c.config != nil {
		return errors.New("config already set")
	}

	if cfg == nil {
		return errors.New("config is nil")
	}

	if cfg.GetMetadata() == nil {
		return errors.New("metadata is nil")
	}

	if cfg.GetMetadata().GetName() == "" {
		return errors.New("name is empty")
	}

	if sboxConfig == nil {
		return errors.New("sandbox config is nil")
	}

	if c.sboxConfig != nil {
		return errors.New("sandbox config is already set")
	}

	c.config = cfg
	c.sboxConfig = sboxConfig

	return nil
}

// SetNameAndID sets a container name and ID.
func (c *container) SetNameAndID(oldID string) error {
	if c.config == nil {
		return errors.New("config is not set")
	}

	if c.sboxConfig == nil {
		return errors.New("sandbox config is nil")
	}

	if c.sboxConfig.GetMetadata() == nil {
		return errors.New("sandbox metadata is nil")
	}

	var id string
	if oldID == "" {
		id = stringid.GenerateNonCryptoID()
	} else {
		id = oldID
	}

	name := strings.Join([]string{
		"k8s",
		c.config.GetMetadata().GetName(),
		c.sboxConfig.GetMetadata().GetName(),
		c.sboxConfig.GetMetadata().GetNamespace(),
		c.sboxConfig.GetMetadata().GetUid(),
		strconv.FormatUint(uint64(c.config.GetMetadata().GetAttempt()), 10),
	}, "_")

	c.id = id
	c.name = name

	return nil
}

// Config returns the container configuration.
func (c *container) Config() *types.ContainerConfig {
	return c.config
}

// SandboxConfig returns the sandbox configuration.
func (c *container) SandboxConfig() *types.PodSandboxConfig {
	return c.sboxConfig
}

// ID returns the container ID.
func (c *container) ID() string {
	return c.id
}

// Name returns the container name.
func (c *container) Name() string {
	return c.name
}

// Restore returns if the container is marked as being
// restored from a checkpoint.
func (c *container) Restore() bool {
	return c.restore
}

// SetRestore marks the container as being restored from a checkpoint.
func (c *container) SetRestore(restore bool) {
	c.restore = restore
}

// SetPrivileged sets the privileged bool for the container.
func (c *container) SetPrivileged() error {
	if c.config == nil {
		return nil
	}

	if c.config.GetLinux() == nil {
		return nil
	}

	if c.config.GetLinux().GetSecurityContext() == nil {
		return nil
	}

	if c.sboxConfig == nil {
		return nil
	}

	if c.sboxConfig.GetLinux() == nil {
		return nil
	}

	if c.sboxConfig.GetLinux().GetSecurityContext() == nil {
		return nil
	}

	if c.config.GetLinux().GetSecurityContext().GetPrivileged() {
		if !c.sboxConfig.GetLinux().GetSecurityContext().GetPrivileged() {
			return errors.New("no privileged container allowed in sandbox")
		}

		c.privileged = true
	}

	return nil
}

// Privileged returns whether this container is privileged.
func (c *container) Privileged() bool {
	return c.privileged
}

// LogPath returns the log path for the container
// It takes as input the LogDir of the sandbox, which is used
// if there is no LogDir configured in the sandbox CRI config.
func (c *container) LogPath(sboxLogDir string) (string, error) {
	sboxLogDirConfig := c.sboxConfig.GetLogDirectory()
	if sboxLogDirConfig != "" {
		sboxLogDir = sboxLogDirConfig
	}

	if sboxLogDir == "" {
		return "", fmt.Errorf("container %s has a sandbox with an empty log path", sboxLogDir)
	}

	logPath := c.config.GetLogPath()
	if logPath == "" {
		logPath = filepath.Join(sboxLogDir, c.ID()+".log")
	} else {
		logPath = filepath.Join(sboxLogDir, logPath)
	}

	// Handle https://issues.k8s.io/44043
	if err := utils.EnsureSaneLogPath(logPath); err != nil {
		return "", err
	}

	logrus.Debugf("Setting container's log_path = %s, sbox.logdir = %s, ctr.logfile = %s",
		sboxLogDir, c.config.GetLogPath(), logPath,
	)

	return logPath, nil
}

// DisableFips returns whether the container should disable fips mode.
func (c *container) DisableFips() bool {
	if value, ok := c.sboxConfig.GetLabels()["FIPS_DISABLE"]; ok && value == "true" {
		return true
	}

	return false
}

// UserRequestedImage returns the image specified in the container spec and used to look up the image when creating the container, or an error.
// The value might evaluate to a different image (or to a different kind of reference!) at any future time.
func (c *container) UserRequestedImage() (string, error) {
	imageSpec := c.config.GetImage()
	if imageSpec == nil {
		return "", errors.New("CreateContainerRequest.ContainerConfig.Image is nil")
	}

	image := imageSpec.GetImage()
	if image == "" {
		return "", errors.New("CreateContainerRequest.ContainerConfig.Image.Image is empty")
	}

	return image, nil
}

// ReadOnly returns whether the rootfs should be readonly
// it takes a bool as to whether crio was configured to
// be readonly, which it defaults to if the container wasn't
// specifically asked to be read only.
func (c *container) ReadOnly(serverIsReadOnly bool) bool {
	if c.config.GetLinux() != nil && c.config.GetLinux().GetSecurityContext().GetReadonlyRootfs() {
		return true
	}

	return serverIsReadOnly
}

// AddUnifiedResourcesFromAnnotations adds the cgroup-v2 resources specified in the io.kubernetes.cri-o.UnifiedCgroup annotation.
func (c *container) AddUnifiedResourcesFromAnnotations(annotationsMap map[string]string) error {
	if c.config == nil || c.config.Labels == nil {
		return nil
	}

	containerName := c.config.GetLabels()[kubeletTypes.KubernetesContainerNameLabel]
	if containerName == "" {
		return nil
	}

	annotationKey := fmt.Sprintf("%s.%s", annotations.UnifiedCgroupAnnotation, containerName)

	annotation := annotationsMap[annotationKey]
	if annotation == "" {
		return nil
	}

	if c.spec.Config.Linux == nil {
		c.spec.Config.Linux = &rspec.Linux{}
	}

	if c.spec.Config.Linux.Resources == nil {
		c.spec.Config.Linux.Resources = &rspec.LinuxResources{}
	}

	if c.spec.Config.Linux.Resources.Unified == nil {
		c.spec.Config.Linux.Resources.Unified = make(map[string]string)
	}

	for r := range strings.SplitSeq(annotation, ";") {
		parts := strings.SplitN(r, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid annotation %q", annotations.UnifiedCgroupAnnotation)
		}

		d, err := b64.StdEncoding.DecodeString(parts[1])
		// if the value is not specified in base64, then use its raw value.
		if err == nil {
			c.spec.Config.Linux.Resources.Unified[parts[0]] = string(d)
		} else {
			c.spec.Config.Linux.Resources.Unified[parts[0]] = parts[1]
		}
	}

	return nil
}

// SpecSetProcessArgs sets the process args in the spec,
// given the image information and passed-in container config.
func (c *container) SpecSetProcessArgs(imageOCIConfig *v1.Image) error {
	kubeCommands := c.config.GetCommand()
	kubeArgs := c.config.GetArgs()

	// merge image config and kube config
	// same as docker does today...
	if imageOCIConfig != nil {
		if len(kubeCommands) == 0 {
			if len(kubeArgs) == 0 {
				kubeArgs = imageOCIConfig.Config.Cmd
			}

			if kubeCommands == nil {
				kubeCommands = imageOCIConfig.Config.Entrypoint
			}
		}
	}

	// create entrypoint and args
	var entrypoint string

	var args []string

	switch {
	case len(kubeCommands) != 0:
		entrypoint = kubeCommands[0]
		args = kubeCommands[1:]
		args = append(args, kubeArgs...)
	case len(kubeArgs) != 0:
		entrypoint = kubeArgs[0]
		args = kubeArgs[1:]
	default:
		return errors.New("no command specified")
	}

	c.spec.SetProcessArgs(append([]string{entrypoint}, args...))

	return nil
}

// WillRunSystemd checks whether the process args
// are configured to be run as a systemd instance.
func (c *container) WillRunSystemd() bool {
	entrypoint := c.spec.Config.Process.Args[0]

	return strings.Contains(entrypoint, "/sbin/init") || (filepath.Base(entrypoint) == "systemd")
}

func (c *container) SpecSetupCapabilities(caps *types.Capability, defaultCaps capabilities.Capabilities, addInheritableCapabilities bool) error {
	// Make sure to remove all ambient capabilities. Kubernetes is not yet ambient capabilities aware
	// and pods expect that switching to a non-root user results in the capabilities being
	// dropped. This should be revisited in the future.
	// Also be sure to remove all inheritable capabilities in accordance with CVE-2022-27652,
	// as it's not idiomatic for a manager of processes to set them.
	specgen := c.Spec()
	// Clear default capabilities from spec
	specgen.ClearProcessCapabilities()

	// Ensure we don't get a nil pointer error if the config
	// doesn't set any capabilities
	if caps == nil {
		caps = &types.Capability{}
	}

	toCAPPrefixed := func(cap string) string {
		if !strings.HasPrefix(strings.ToLower(cap), "cap_") {
			return "CAP_" + strings.ToUpper(cap)
		}

		return cap
	}

	addAll := inStringSlice(caps.GetAddCapabilities(), "ALL")
	dropAll := inStringSlice(caps.GetDropCapabilities(), "ALL")

	// Only add the default capabilities to the AddCapabilities list
	// if neither add or drop are set to "ALL". If add is set to "ALL" it
	// is a super set of the default capabilities. If drop is set to "ALL"
	// then we first want to clear the entire list (including defaults)
	// so the user may selectively add *only* the capabilities they need.
	if !addAll && !dropAll {
		caps.AddCapabilities = append(caps.AddCapabilities, defaultCaps...)
	}

	capabilitiesList, err := getOCICapabilitiesList()
	if err != nil {
		return fmt.Errorf("get OCI capabilities list: %w", err)
	}

	// Add/drop all capabilities if "all" is specified, so that
	// following individual add/drop could still work. E.g.
	// AddCapabilities: []string{"ALL"}, DropCapabilities: []string{"CHOWN"}
	// will be all capabilities without `CAP_CHOWN`.
	// see https://github.com/kubernetes/kubernetes/issues/51980
	if addAll {
		for _, c := range capabilitiesList {
			if err := specgen.AddProcessCapabilityBounding(c); err != nil {
				return err
			}

			if err := specgen.AddProcessCapabilityEffective(c); err != nil {
				return err
			}

			if err := specgen.AddProcessCapabilityPermitted(c); err != nil {
				return err
			}

			if addInheritableCapabilities {
				if err := specgen.AddProcessCapabilityInheritable(c); err != nil {
					return err
				}
			}
		}
	}

	if dropAll {
		for _, c := range capabilitiesList {
			if err := specgen.DropProcessCapabilityBounding(c); err != nil {
				return err
			}

			if err := specgen.DropProcessCapabilityEffective(c); err != nil {
				return err
			}

			if err := specgen.DropProcessCapabilityPermitted(c); err != nil {
				return err
			}

			if addInheritableCapabilities {
				if err := specgen.DropProcessCapabilityInheritable(c); err != nil {
					return err
				}
			}
		}
	}

	for _, cap := range caps.GetAddCapabilities() {
		if strings.EqualFold(cap, "ALL") {
			continue
		}

		capPrefixed := toCAPPrefixed(cap)
		// Validate capability
		if !inStringSlice(capabilitiesList, capPrefixed) {
			return fmt.Errorf("unknown capability %q to add", capPrefixed)
		}

		if err := specgen.AddProcessCapabilityBounding(capPrefixed); err != nil {
			return err
		}

		if err := specgen.AddProcessCapabilityEffective(capPrefixed); err != nil {
			return err
		}

		if err := specgen.AddProcessCapabilityPermitted(capPrefixed); err != nil {
			return err
		}

		if addInheritableCapabilities {
			if err := specgen.AddProcessCapabilityInheritable(capPrefixed); err != nil {
				return err
			}
		}
	}

	for _, cap := range caps.GetDropCapabilities() {
		if strings.EqualFold(cap, "ALL") {
			continue
		}

		capPrefixed := toCAPPrefixed(cap)
		if err := specgen.DropProcessCapabilityBounding(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %w", capPrefixed, err)
		}

		if err := specgen.DropProcessCapabilityEffective(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %w", capPrefixed, err)
		}

		if err := specgen.DropProcessCapabilityPermitted(capPrefixed); err != nil {
			return fmt.Errorf("failed to drop cap %s %w", capPrefixed, err)
		}

		if addInheritableCapabilities {
			if err := specgen.DropProcessCapabilityInheritable(capPrefixed); err != nil {
				return err
			}
		}
	}

	return nil
}

// inStringSlice checks whether a string is inside a string slice.
// Comparison is case insensitive.
func inStringSlice(ss []string, str string) bool {
	for _, s := range ss {
		if strings.EqualFold(s, str) {
			return true
		}
	}

	return false
}

// getOCICapabilitiesList returns a list of all available capabilities.
func getOCICapabilitiesList() ([]string, error) {
	caps := make([]string, 0, len(capability.ListKnown()))

	lastCap, err := capability.LastCap()
	if err != nil {
		return nil, fmt.Errorf("get last capability: %w", err)
	}

	for _, cap := range capability.ListKnown() {
		if cap > lastCap {
			continue
		}

		caps = append(caps, "CAP_"+strings.ToUpper(cap.String()))
	}

	return caps, nil
}

func (c *container) SpecSetPrivileges(ctx context.Context, securityContext *types.LinuxContainerSecurityContext, cfg *config.Config) error {
	specgen := c.Spec()
	if c.Privileged() {
		specgen.SetupPrivileged(true)
	} else {
		caps := securityContext.GetCapabilities()
		if err := c.SpecSetupCapabilities(caps, cfg.DefaultCapabilities, cfg.AddInheritableCapabilities); err != nil {
			return err
		}
	}

	if securityContext.GetNoNewPrivs() {
		const sysAdminCap = "CAP_SYS_ADMIN"
		for _, cap := range specgen.Config.Process.Capabilities.Bounding {
			if cap == sysAdminCap {
				log.Warnf(ctx, "Setting `noNewPrivileges` flag has no effect because container has %s capability", sysAdminCap)
			}
		}

		if c.Privileged() {
			log.Warnf(ctx, "Setting `noNewPrivileges` flag has no effect because container is privileged")
		}
	}

	specgen.SetProcessNoNewPrivileges(securityContext.GetNoNewPrivs())

	if !c.Privileged() {
		if securityContext.MaskedPaths != nil {
			for _, path := range securityContext.GetMaskedPaths() {
				specgen.AddLinuxMaskedPaths(path)
			}
		}

		if securityContext.ReadonlyPaths != nil {
			for _, path := range securityContext.GetReadonlyPaths() {
				specgen.AddLinuxReadonlyPaths(path)
			}
		}
	}

	return nil
}

func (c *container) SpecSetLinuxContainerResources(resources *types.LinuxContainerResources, containerMinMemory int64) error {
	specgen := c.Spec()
	specgen.SetLinuxResourcesCPUPeriod(uint64(resources.GetCpuPeriod()))
	specgen.SetLinuxResourcesCPUQuota(resources.GetCpuQuota())
	specgen.SetLinuxResourcesCPUShares(uint64(resources.GetCpuShares()))

	memoryLimit := resources.GetMemoryLimitInBytes()
	if memoryLimit != 0 {
		if err := cgmgr.VerifyMemoryIsEnough(memoryLimit, containerMinMemory); err != nil {
			return err
		}

		specgen.SetLinuxResourcesMemoryLimit(memoryLimit)

		if resources.GetMemorySwapLimitInBytes() != 0 {
			if resources.GetMemorySwapLimitInBytes() > 0 && resources.GetMemorySwapLimitInBytes() < resources.GetMemoryLimitInBytes() {
				return fmt.Errorf(
					"container %s create failed because memory swap limit (%d) cannot be lower than memory limit (%d)",
					c.ID(),
					resources.GetMemorySwapLimitInBytes(),
					resources.GetMemoryLimitInBytes(),
				)
			}

			memoryLimit = resources.GetMemorySwapLimitInBytes()
		}
		// If node doesn't have memory swap, then skip setting
		// otherwise the container creation fails.
		if node.CgroupHasMemorySwap() {
			specgen.SetLinuxResourcesMemorySwap(memoryLimit)
		}
	}

	specgen.SetProcessOOMScoreAdj(int(resources.GetOomScoreAdj()))
	specgen.SetLinuxResourcesCPUCpus(resources.GetCpusetCpus())
	specgen.SetLinuxResourcesCPUMems(resources.GetCpusetMems())

	// If the kernel has no support for hugetlb, silently ignore the limits
	if node.CgroupHasHugetlb() {
		hugepageLimits := resources.GetHugepageLimits()
		for _, limit := range hugepageLimits {
			specgen.AddLinuxResourcesHugepageLimit(limit.GetPageSize(), limit.GetLimit())
		}
	}

	if node.CgroupIsV2() && len(resources.GetUnified()) != 0 {
		if specgen.Config.Linux.Resources.Unified == nil {
			specgen.Config.Linux.Resources.Unified = make(map[string]string, len(resources.GetUnified()))
		}

		maps.Copy(specgen.Config.Linux.Resources.Unified, resources.GetUnified())
	}

	return nil
}
