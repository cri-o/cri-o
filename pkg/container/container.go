package container

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/containers/podman/v2/pkg/annotations"
	"github.com/containers/storage/pkg/stringid"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	oci "github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/utils"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// Container is the main public container interface
type Container interface {
	// All set methods are usually called in order of their definition

	// SetConfig sets the configuration to the container and validates it
	SetConfig(*pb.ContainerConfig, *pb.PodSandboxConfig) error

	// SetNameAndID sets a container name and ID
	SetNameAndID() error

	// Config returns the container CRI configuration
	Config() *pb.ContainerConfig

	// SandboxConfig returns the sandbox CRI configuration
	SandboxConfig() *pb.PodSandboxConfig

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

	// Image returns the image specified in the container spec, or an error
	Image() (string, error)

	// ReadOnly returns whether the rootfs should be readonly
	// it takes a bool as to whether crio was configured to
	// be readonly, which it defaults to if the container wasn't
	// specifically asked to be read only
	ReadOnly(bool) bool

	// SelinuxLabel returns the container's SelinuxLabel
	// it takes the sandbox's label, which it falls back upon
	SelinuxLabel(string) ([]string, error)

	// spec functions

	// returns the spec
	Spec() *generate.Generator

	// SpecAddMount adds a mount to the container's spec
	// it takes the rspec mount object
	// if there is already a mount at the path specified, it removes it.
	SpecAddMount(rspec.Mount)

	// SpecAddAnnotations adds annotations to the spec.
	SpecAddAnnotations(sandbox *sandbox.Sandbox, containerVolume []oci.ContainerVolume, mountPoint, configStopSignal string, imageResult *storage.ImageResult, isSystemd, systemdHasCollectMode bool) error
}

// container is the hidden default type behind the Container interface
type container struct {
	ctx        context.Context
	config     *pb.ContainerConfig
	sboxConfig *pb.PodSandboxConfig
	id         string
	name       string
	privileged bool
	spec       generate.Generator
}

// New creates a new, empty Sandbox instance
func New(ctx context.Context) (Container, error) {
	spec, err := generate.New("linux")
	if err != nil {
		return nil, err
	}
	return &container{
		ctx:  ctx,
		spec: spec,
	}, nil
}

// SpecAddMount adds a specified mount to the spec
func (c *container) SpecAddMount(r rspec.Mount) {
	c.spec.RemoveMount(r.Destination)
	c.spec.AddMount(r)
}

// SpecAddAnnotation adds all annotations to the spec
func (c *container) SpecAddAnnotations(sb *sandbox.Sandbox, containerVolumes []oci.ContainerVolume, mountPoint, configStopSignal string, imageResult *storage.ImageResult, isSystemd, systemdHasCollectMode bool) (err error) {
	// Copied from k8s.io/kubernetes/pkg/kubelet/kuberuntime/labels.go
	const podTerminationGracePeriodLabel = "io.kubernetes.pod.terminationGracePeriod"

	kubeAnnotations := c.Config().GetAnnotations()
	created := time.Now()
	labels := c.Config().GetLabels()

	image, err := c.Image()
	if err != nil {
		return err
	}
	logPath, err := c.LogPath(sb.LogDir())
	if err != nil {
		return err
	}
	c.spec.AddAnnotation(annotations.Image, image)
	c.spec.AddAnnotation(annotations.ImageName, imageResult.Name)
	c.spec.AddAnnotation(annotations.ImageRef, imageResult.ID)
	c.spec.AddAnnotation(annotations.Name, c.Name())
	c.spec.AddAnnotation(annotations.ContainerID, c.ID())
	c.spec.AddAnnotation(annotations.SandboxID, sb.ID())
	c.spec.AddAnnotation(annotations.SandboxName, sb.Name())
	c.spec.AddAnnotation(annotations.ContainerType, annotations.ContainerTypeContainer)
	c.spec.AddAnnotation(annotations.LogPath, logPath)
	c.spec.AddAnnotation(annotations.TTY, strconv.FormatBool(c.Config().Tty))
	c.spec.AddAnnotation(annotations.Stdin, strconv.FormatBool(c.Config().Stdin))
	c.spec.AddAnnotation(annotations.StdinOnce, strconv.FormatBool(c.Config().StdinOnce))
	c.spec.AddAnnotation(annotations.ResolvPath, sb.ResolvPath())
	c.spec.AddAnnotation(annotations.ContainerManager, lib.ContainerManagerCRIO)
	c.spec.AddAnnotation(annotations.MountPoint, mountPoint)
	c.spec.AddAnnotation(annotations.SeccompProfilePath, c.Config().GetLinux().GetSecurityContext().GetSeccompProfilePath())
	c.spec.AddAnnotation(annotations.Created, created.Format(time.RFC3339Nano))

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
		if systemdHasCollectMode {
			c.spec.AddAnnotation("org.systemd.property.CollectMode", "'inactive-or-failed'")
		}
	}

	if configStopSignal != "" {
		// this key is defined in image-spec conversion document at https://github.com/opencontainers/image-spec/pull/492/files#diff-8aafbe2c3690162540381b8cdb157112R57
		c.spec.AddAnnotation("org.opencontainers.image.stopSignal", configStopSignal)
	}

	return nil
}

func (c *container) Spec() *generate.Generator {
	return &c.spec
}

// SetConfig sets the configuration to the container and validates it
func (c *container) SetConfig(config *pb.ContainerConfig, sboxConfig *pb.PodSandboxConfig) error {
	if c.config != nil {
		return errors.New("config already set")
	}

	if config == nil {
		return errors.New("config is nil")
	}

	if config.GetMetadata() == nil {
		return errors.New("metadata is nil")
	}

	if config.GetMetadata().GetName() == "" {
		return errors.New("name is nil")
	}

	if sboxConfig == nil {
		return errors.New("sandbox config is nil")
	}

	if c.sboxConfig != nil {
		return errors.New("sandbox config is already set")
	}

	c.config = config
	c.sboxConfig = sboxConfig
	return nil
}

// SetNameAndID sets a container name and ID
func (c *container) SetNameAndID() error {
	if c.config == nil {
		return errors.New("config is not set")
	}

	if c.sboxConfig == nil {
		return errors.New("sandbox config is nil")
	}

	if c.sboxConfig.Metadata == nil {
		return errors.New("sandbox metadata is nil")
	}

	id := stringid.GenerateNonCryptoID()
	name := strings.Join([]string{
		"k8s",
		c.config.Metadata.Name,
		c.sboxConfig.Metadata.Name,
		c.sboxConfig.Metadata.Namespace,
		c.sboxConfig.Metadata.Uid,
		fmt.Sprintf("%d", c.config.Metadata.Attempt),
	}, "_")

	c.id = id
	c.name = name
	return nil
}

// Config returns the container configuration
func (c *container) Config() *pb.ContainerConfig {
	return c.config
}

// SandboxConfig returns the sandbox configuration
func (c *container) SandboxConfig() *pb.PodSandboxConfig {
	return c.sboxConfig
}

// ID returns the container ID
func (c *container) ID() string {
	return c.id
}

// Name returns the container name
func (c *container) Name() string {
	return c.name
}

// SetPrivileged sets the privileged bool for the container
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

// Privileged returns whether this container is privileged
func (c *container) Privileged() bool {
	return c.privileged
}

// LogPath returns the log path for the container
// It takes as input the LogDir of the sandbox, which is used
// if there is no LogDir configured in the sandbox CRI config
func (c *container) LogPath(sboxLogDir string) (string, error) {
	sboxLogDirConfig := c.sboxConfig.GetLogDirectory()
	if sboxLogDirConfig != "" {
		sboxLogDir = sboxLogDirConfig
	}

	if sboxLogDir == "" {
		return "", errors.Errorf("container %s has a sandbox with an empty log path", sboxLogDir)
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

	logrus.Debugf("setting container's log_path = %s, sbox.logdir = %s, ctr.logfile = %s",
		sboxLogDir, c.config.GetLogPath(), logPath,
	)
	return logPath, nil
}

// DisableFips returns whether the container should disable fips mode
func (c *container) DisableFips() bool {
	if value, ok := c.sboxConfig.GetLabels()["FIPS_DISABLE"]; ok && value == "true" {
		return true
	}
	return false
}

// Image returns the image specified in the container spec, or an error
func (c *container) Image() (string, error) {
	imageSpec := c.config.GetImage()
	if imageSpec == nil {
		return "", errors.New("CreateContainerRequest.ContainerConfig.Image is nil")
	}

	image := imageSpec.Image
	if image == "" {
		return "", errors.New("CreateContainerRequest.ContainerConfig.Image.Image is empty")
	}
	return image, nil
}

// ReadOnly returns whether the rootfs should be readonly
// it takes a bool as to whether crio was configured to
// be readonly, which it defaults to if the container wasn't
// specifically asked to be read only
func (c *container) ReadOnly(serverIsReadOnly bool) bool {
	if c.config.GetLinux().GetSecurityContext().GetReadonlyRootfs() {
		return true
	}
	return serverIsReadOnly
}

// SelinuxLabel returns the container's SelinuxLabel
// it takes the sandbox's label, which it falls back upon
func (c *container) SelinuxLabel(sboxLabel string) ([]string, error) {
	selinuxConfig := c.config.GetLinux().GetSecurityContext().GetSelinuxOptions()

	labels := map[string]string{}

	labelOptions, err := label.DupSecOpt(sboxLabel)
	if err != nil {
		return nil, err
	}
	for _, r := range labelOptions {
		k := strings.Split(r, ":")[0]
		labels[k] = r
	}

	if selinuxConfig != nil {
		for _, r := range utils.GetLabelOptions(selinuxConfig) {
			k := strings.Split(r, ":")[0]
			labels[k] = r
		}
	}
	ret := []string{}
	for _, v := range labels {
		ret = append(ret, v)
	}
	return ret, nil
}
