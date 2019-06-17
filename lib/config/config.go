package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/containers/image/pkg/sysregistriesv2"
	"github.com/containers/image/types"
	"github.com/containers/libpod/pkg/rootless"
	createconfig "github.com/containers/libpod/pkg/spec"
	"github.com/containers/storage"
	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/utils"
	units "github.com/docker/go-units"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Defaults if none are specified
const (
	pauseImage          = "k8s.gcr.io/pause:3.1"
	pauseCommand        = "/pause"
	defaultTransport    = "docker://"
	apparmorProfileName = "crio-default"
	defaultRuntime      = "runc"
	defaultRuntimePath  = "/usr/bin/runc"
	defaultRuntimeType  = "oci"
	defaultRuntimeRoot  = "/run/runc"
	cgroupManager       = "cgroupfs"
)

// Config represents the entire set of configuration values that can be set for
// the server. This is intended to be loaded from a toml-encoded config file.
type Config struct {
	RootConfig
	RuntimeConfig
	ImageConfig
	NetworkConfig
}

// Iface provides a config interface for data encapsulation
type Iface interface {
	GetStore() (cstorage.Store, error)
	GetData() *Config
}

// GetStore returns the container storage for a given configuration
func (c *Config) GetStore() (cstorage.Store, error) {
	return cstorage.GetStore(cstorage.StoreOptions{
		RunRoot:            c.RunRoot,
		GraphRoot:          c.Root,
		GraphDriverName:    c.Storage,
		GraphDriverOptions: c.StorageOptions,
	})
}

// GetData returns the Config of a Iface
func (c *Config) GetData() *Config {
	return c
}

// ImageVolumesType describes image volume handling strategies
type ImageVolumesType string

const (
	// ImageVolumesMkdir option is for using mkdir to handle image volumes
	ImageVolumesMkdir ImageVolumesType = "mkdir"
	// ImageVolumesIgnore option is for ignoring image volumes altogether
	ImageVolumesIgnore ImageVolumesType = "ignore"
	// ImageVolumesBind option is for using bind mounted volumes
	ImageVolumesBind ImageVolumesType = "bind"
)

const (
	// DefaultPidsLimit is the default value for maximum number of processes
	// allowed inside a container
	DefaultPidsLimit = 1024

	// DefaultLogSizeMax is the default value for the maximum log size
	// allowed for a container. Negative values mean that no limit is imposed.
	DefaultLogSizeMax = -1

	// DefaultLogToJournald is the default value for whether conmon should
	// log to journald in addition to kubernetes log file.
	DefaultLogToJournald = false
)

// DefaultCapabilities for the default_capabilities option in the crio.conf file
var DefaultCapabilities = []string{
	"CHOWN",
	"DAC_OVERRIDE",
	"FSETID",
	"FOWNER",
	"NET_RAW",
	"SETGID",
	"SETUID",
	"SETPCAP",
	"NET_BIND_SERVICE",
	"SYS_CHROOT",
	"KILL",
}

// This structure is necessary to fake the TOML tables when parsing,
// while also not requiring a bunch of layered structs for no good
// reason.

// RootConfig represents the root of the "crio" TOML config table.
type RootConfig struct {
	// Root is a path to the "root directory" where data not
	// explicitly handled by other options will be stored.
	Root string `toml:"root"`

	// RunRoot is a path to the "run directory" where state information not
	// explicitly handled by other options will be stored.
	RunRoot string `toml:"runroot"`

	// Storage is the name of the storage driver which handles actually
	// storing the contents of containers.
	Storage string `toml:"storage_driver"`

	// StorageOption is a list of storage driver specific options.
	StorageOptions []string `toml:"storage_option"`

	// LogDir is the default log directory where all logs will go unless kubelet
	// tells us to put them somewhere else.
	LogDir string `toml:"log_dir"`

	// FileLocking specifies whether to use file-based or in-memory locking
	// File-based locking is required when multiple users of lib are
	// present on the same system
	FileLocking bool `toml:"file_locking"`

	// FileLockingPath specifies the path to use for the locking.
	FileLockingPath string `toml:"file_locking_path"`
}

// RuntimeHandler represents each item of the "crio.runtime.runtimes" TOML
// config table.
type RuntimeHandler struct {
	RuntimePath string `toml:"runtime_path"`
	RuntimeType string `toml:"runtime_type"`
	RuntimeRoot string `toml:"runtime_root"`
}

// RuntimeConfig represents the "crio.runtime" TOML config table.
type RuntimeConfig struct {
	// ConmonEnv is the environment variable list for conmon process.
	ConmonEnv []string `toml:"conmon_env"`

	// HooksDir holds paths to the directories containing hooks
	// configuration files.  When the same filename is present in in
	// multiple directories, the file in the directory listed last in
	// this slice takes precedence.
	HooksDir []string `toml:"hooks_dir"`

	// DefaultMounts is the list of mounts to be mounted for each container
	// The format of each mount is "host-path:container-path"
	DefaultMounts []string `toml:"default_mounts"`

	// Capabilities to add to all containers.
	DefaultCapabilities []string `toml:"default_capabilities"`

	// Sysctls to add to all containers.
	DefaultSysctls []string `toml:"default_sysctls"`

	// DefaultUlimits specifies the default ulimits to apply to containers
	DefaultUlimits []string `toml:"default_ulimits"`

	// Devices to add to containers
	AdditionalDevices []string `toml:"additional_devices"`

	// DefaultRuntime is the _name_ of the OCI runtime to be used as the default.
	// The name is matched against the Runtimes map below.
	DefaultRuntime string `toml:"default_runtime"`

	// Conmon is the path to conmon binary, used for managing the runtime.
	Conmon string `toml:"conmon"`

	// ConmonCgroup is the cgroup setting used for conmon.
	ConmonCgroup string `toml:"conmon_cgroup"`

	// SeccompProfile is the seccomp json profile path which is used as the
	// default for the runtime.
	SeccompProfile string `toml:"seccomp_profile"`

	// ApparmorProfile is the apparmor profile name which is used as the
	// default for the runtime.
	ApparmorProfile string `toml:"apparmor_profile"`

	// CgroupManager is the manager implementation name which is used to
	// handle cgroups for containers.
	CgroupManager string `toml:"cgroup_manager"`

	// DefaultMountsFile is the file path for the default mounts to be mounted for the container
	// Note, for testing purposes mainly
	DefaultMountsFile string `toml:"default_mounts_file"`

	// ContainerExitsDir is the directory in which container exit files are
	// written to by conmon.
	ContainerExitsDir string `toml:"container_exits_dir"`

	// ContainerAttachSocketDir is the location for container attach sockets.
	ContainerAttachSocketDir string `toml:"container_attach_socket_dir"`

	// BindMountPrefix is the prefix to use for the source of the bind mounts.
	BindMountPrefix string `toml:"bind_mount_prefix"`

	// UIDMappings specifies the UID mappings to have in the user namespace.
	// A range is specified in the form containerUID:HostUID:Size.  Multiple
	// ranges are separated by comma.
	UIDMappings string `toml:"uid_mappings"`

	// GIDMappings specifies the GID mappings to have in the user namespace.
	// A range is specified in the form containerUID:HostUID:Size.  Multiple
	// ranges are separated by comma.
	GIDMappings string `toml:"gid_mappings"`

	// LogLevel determines the verbosity of the logs based on the level it is set to.
	// Options are fatal, panic, error (default), warn, info, and debug.
	LogLevel string `toml:"log_level"`

	// Runtimes defines a list of OCI compatible runtimes. The runtime to
	// use is picked based on the runtime_handler provided by the CRI. If
	// no runtime_handler is provided, the runtime will be picked based on
	// the level of trust of the workload.
	Runtimes map[string]RuntimeHandler `toml:"runtimes"`

	// PidsLimit is the number of processes each container is restricted to
	// by the cgroup process number controller.
	PidsLimit int64 `toml:"pids_limit"`

	// LogSizeMax is the maximum number of bytes after which the log file
	// will be truncated. It can be expressed as a human-friendly string
	// that is parsed to bytes.
	// Negative values indicate that the log file won't be truncated.
	LogSizeMax int64 `toml:"log_size_max"`

	// CtrStopTimeout specifies the time to wait before to generate an
	// error because the container state is still tagged as "running".
	CtrStopTimeout int64 `toml:"ctr_stop_timeout"`

	// NoPivot instructs the runtime to not use `pivot_root`, but instead use `MS_MOVE`
	NoPivot bool `toml:"no_pivot"`

	// SELinux determines whether or not SELinux is used for pod separation.
	SELinux bool `toml:"selinux"`

	// Whether container output should be logged to journald in addition
	// to the kuberentes log file
	LogToJournald bool `toml:"log_to_journald"`

	// ManageNetworkNSLifecycle determines whether we pin and remove network namespace
	// and manage its lifecycle
	ManageNetworkNSLifecycle bool `toml:"manage_network_ns_lifecycle"`

	// ReadOnly run all pods/containers in read-only mode.
	// This mode will mount tmpfs on /run, /tmp and /var/tmp, if those are not mountpoints
	// Will also set the readonly flag in the OCI Runtime Spec.  In this mode containers
	// will only be able to write to volumes mounted into them
	ReadOnly bool `toml:"read_only"`
}

// ImageConfig represents the "crio.image" TOML config table.
type ImageConfig struct {
	// DefaultTransport is a value we prefix to image names that fail to
	// validate source references.
	DefaultTransport string `toml:"default_transport"`
	// PauseImage is the name of an image which we use to instantiate infra
	// containers.
	PauseImage string `toml:"pause_image"`
	// PauseImageAuthFile, if not empty, is a path to a docker/config.json-like
	// file containing credentials necessary for pulling PauseImage
	PauseImageAuthFile string `toml:"pause_image_auth_file"`
	// PauseCommand is the path of the binary we run in an infra
	// container that's been instantiated using PauseImage.
	PauseCommand string `toml:"pause_command"`
	// SignaturePolicyPath is the name of the file which decides what sort
	// of policy we use when deciding whether or not to trust an image that
	// we've pulled.  Outside of testing situations, it is strongly advised
	// that this be left unspecified so that the default system-wide policy
	// will be used.
	SignaturePolicyPath string `toml:"signature_policy"`
	// InsecureRegistries is a list of registries that must be contacted w/o
	// TLS verification.
	InsecureRegistries []string `toml:"insecure_registries"`
	// ImageVolumes controls how volumes specified in image config are handled
	ImageVolumes ImageVolumesType `toml:"image_volumes"`
	// Registries holds a list of registries used to pull unqualified images
	Registries []string `toml:"registries"`
}

// NetworkConfig represents the "crio.network" TOML config table
type NetworkConfig struct {
	// NetworkDir is where CNI network configuration files are stored.
	NetworkDir string `toml:"network_dir"`

	// PluginDir is where CNI plugin binaries are stored.
	PluginDir string `toml:"plugin_dir,omitempty"`

	// PluginDirs is where CNI plugin binaries are stored.
	PluginDirs []string `toml:"plugin_dirs"`
}

// tomlConfig is another way of looking at a Config, which is
// TOML-friendly (it has all of the explicit tables). It's just used for
// conversions.
type tomlConfig struct {
	Crio struct {
		RootConfig
		Runtime struct{ RuntimeConfig } `toml:"runtime"`
		Image   struct{ ImageConfig }   `toml:"image"`
		Network struct{ NetworkConfig } `toml:"network"`
	} `toml:"crio"`
}

func (t *tomlConfig) toConfig(c *Config) {
	c.RootConfig = t.Crio.RootConfig
	c.RuntimeConfig = t.Crio.Runtime.RuntimeConfig
	c.ImageConfig = t.Crio.Image.ImageConfig
	c.NetworkConfig = t.Crio.Network.NetworkConfig
}

func (t *tomlConfig) fromConfig(c *Config) {
	t.Crio.RootConfig = c.RootConfig
	t.Crio.Runtime.RuntimeConfig = c.RuntimeConfig
	t.Crio.Image.ImageConfig = c.ImageConfig
	t.Crio.Network.NetworkConfig = c.NetworkConfig
}

// UpdateFromFile populates the Config from the TOML-encoded file at the given path.
// Returns errors encountered when reading or parsing the files, or nil
// otherwise.
func (c *Config) UpdateFromFile(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	t := new(tomlConfig)
	t.fromConfig(c)

	_, err = toml.Decode(string(data), t)
	if err != nil {
		return fmt.Errorf("unable to decode configuration %v: %v", path, err)
	}

	t.toConfig(c)
	return nil
}

// ToFile outputs the given Config as a TOML-encoded file at the given path.
// Returns errors encountered when generating or writing the file, or nil
// otherwise.
func (c *Config) ToFile(path string) error {
	var w bytes.Buffer
	e := toml.NewEncoder(&w)

	t := new(tomlConfig)
	t.fromConfig(c)

	if err := e.Encode(*t); err != nil {
		return err
	}

	return ioutil.WriteFile(path, w.Bytes(), 0644)
}

// DefaultConfig returns the default configuration for crio.
func DefaultConfig(systemContext *types.SystemContext) (*Config, error) {
	registries, err := sysregistriesv2.UnqualifiedSearchRegistries(systemContext)
	if err != nil {
		registries = nil // Ignore the error otherwise
	}
	insecureRegistries := []string{}
	allRegistries, err := sysregistriesv2.GetRegistries(systemContext)
	if err == nil { // Ignore the error otherwise
		for _, reg := range allRegistries {
			if reg.Insecure {
				insecureRegistries = append(insecureRegistries, reg.Prefix)
			}
		}
	}
	storeOpts, err := storage.DefaultStoreOptions(rootless.IsRootless(), rootless.GetRootlessUID())
	if err != nil {
		return nil, err
	}
	return &Config{
		RootConfig: RootConfig{
			Root:            storeOpts.GraphRoot,
			RunRoot:         storeOpts.RunRoot,
			Storage:         storeOpts.GraphDriverName,
			StorageOptions:  storeOpts.GraphDriverOptions,
			LogDir:          "/var/log/crio/pods",
			FileLocking:     true,
			FileLockingPath: lockPath,
		},
		RuntimeConfig: RuntimeConfig{
			DefaultRuntime: defaultRuntime,
			Runtimes: map[string]RuntimeHandler{
				defaultRuntime: {
					RuntimePath: defaultRuntimePath,
					RuntimeType: defaultRuntimeType,
					RuntimeRoot: defaultRuntimeRoot,
				},
			},
			Conmon: conmonPath,
			ConmonEnv: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
			ConmonCgroup:             "pod",
			SELinux:                  selinuxEnabled(),
			SeccompProfile:           seccompProfilePath,
			ApparmorProfile:          apparmorProfileName,
			CgroupManager:            cgroupManager,
			PidsLimit:                DefaultPidsLimit,
			ContainerExitsDir:        containerExitsDir,
			ContainerAttachSocketDir: ContainerAttachSocketDir,
			LogSizeMax:               DefaultLogSizeMax,
			LogToJournald:            DefaultLogToJournald,
			DefaultCapabilities:      DefaultCapabilities,
			LogLevel:                 "error",
			DefaultSysctls:           []string{},
			DefaultUlimits:           []string{},
			AdditionalDevices:        []string{},
		},
		ImageConfig: ImageConfig{
			DefaultTransport:    defaultTransport,
			PauseImage:          pauseImage,
			PauseImageAuthFile:  "",
			PauseCommand:        pauseCommand,
			SignaturePolicyPath: "",
			ImageVolumes:        ImageVolumesMkdir,
			Registries:          registries,
			InsecureRegistries:  insecureRegistries,
		},
		NetworkConfig: NetworkConfig{
			NetworkDir: cniConfigDir,
			PluginDirs: []string{cniBinDir},
		},
	}, nil
}

// Validate is the main entry point for library configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *Config) Validate(systemContext *types.SystemContext, onExecution bool) error {
	if err := c.RootConfig.Validate(onExecution); err != nil {
		return errors.Wrapf(err, "root config")
	}

	if err := c.RuntimeConfig.Validate(systemContext, onExecution); err != nil {
		return errors.Wrapf(err, "runtime config")
	}

	if err := c.NetworkConfig.Validate(onExecution); err != nil {
		return errors.Wrapf(err, "network config")
	}

	return nil
}

// Validate is the main entry point for root configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *RootConfig) Validate(onExecution bool) error {
	if onExecution {
		if err := os.MkdirAll(c.LogDir, 0700); err != nil {
			return errors.Wrapf(err, "invalid log_dir")
		}
	}

	return nil
}

// Validate is the main entry point for runtime configuration validation
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *RuntimeConfig) Validate(systemContext *types.SystemContext, onExecution bool) error {
	// This is somehow duplicated with server.getUlimitsFromConfig under server/utils.go
	// but I don't want to export that function for the sake of validation here
	// so, keep it in mind if things start to blow up.
	// Reason for having this here is that I don't want people to start crio
	// with invalid ulimits but realize that only after starting a couple of
	// containers and watching them fail.
	for _, u := range c.DefaultUlimits {
		ul, err := units.ParseUlimit(u)
		if err != nil {
			return fmt.Errorf("unrecognized ulimit %s: %v", u, err)
		}
		_, err = ul.GetRlimit()
		if err != nil {
			return err
		}
	}

	for _, d := range c.AdditionalDevices {
		split := strings.Split(d, ":")
		switch len(split) {
		case 3:
			if !createconfig.IsValidDeviceMode(split[2]) {
				return fmt.Errorf("invalid device mode: %s", split[2])
			}
			fallthrough
		case 2:
			if (!createconfig.IsValidDeviceMode(split[1]) && !strings.HasPrefix(split[1], "/dev/")) ||
				(len(split) == 3 && createconfig.IsValidDeviceMode(split[1])) {
				return fmt.Errorf("invalid device mode: %s", split[1])
			}
			fallthrough
		case 1:
			if !strings.HasPrefix(split[0], "/dev/") {
				return fmt.Errorf("invalid device mode: %s", split[0])
			}
		default:
			return fmt.Errorf("invalid device specification: %s", d)
		}
	}

	// check we do have at least a runtime
	if _, ok := c.Runtimes[c.DefaultRuntime]; !ok {
		// Set the default runtime to "runc" if default_runtime is not set
		if c.DefaultRuntime == "" {
			logrus.Debugf("Defaulting to %q as the runtime since default_runtime is not set", defaultRuntime)
			// The default config sets runc and its path in the runtimes map, so check for that
			// first. If it does not exist then we add runc + its path to the runtimes map.
			if _, ok := c.Runtimes[defaultRuntime]; !ok {
				c.Runtimes[defaultRuntime] = oci.RuntimeHandler{RuntimePath: defaultRuntimePath, RuntimeType: defaultRuntimeType, RuntimeRoot: defaultRuntimeRoot}
			}
			// Set the DefaultRuntime to runc so we don't fail further along in the code
			c.DefaultRuntime = defaultRuntime
		} else {
			return fmt.Errorf("default_runtime set to %q, but no runtime path is set for it", c.DefaultRuntime)
		}
	}

	if !(c.ConmonCgroup == "pod" || strings.HasSuffix(c.ConmonCgroup, ".slice")) {
		return errors.New("conmon cgroup should be 'pod' or a systemd slice")
	}

	// check for validation on execution
	if onExecution {
		// Validate if runtime_path does exist for each runtime
		for runtime, handler := range c.Runtimes {
			if _, err := os.Stat(handler.RuntimePath); os.IsNotExist(err) {
				return fmt.Errorf("invalid runtime_path for runtime '%s': %q",
					runtime, err)
			}
			logrus.Debugf("found valid runtime '%s' for runtime_path '%s'\n",
				runtime, handler.RuntimePath)
		}

		// Validate the system registries configuration
		if _, err := sysregistriesv2.GetRegistries(systemContext); err != nil {
			return errors.Wrapf(err, "invalid registries")
		}

		for _, hooksDir := range c.HooksDir {
			if err := utils.IsDirectory(hooksDir); err != nil {
				return errors.Wrapf(err, "invalid hooks_dir: %s", err)
			}
		}

		if _, err := os.Stat(c.Conmon); err != nil {
			return errors.Wrapf(err, "invalid conmon path")
		}
	}

	return nil
}

// Validate is the main entry point for network configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *NetworkConfig) Validate(onExecution bool) error {
	if onExecution {
		err := utils.IsDirectory(c.NetworkDir)
		if err != nil {
			if os.IsNotExist(err) {
				if err = os.MkdirAll(c.NetworkDir, 0755); err != nil {
					return errors.Wrapf(err, "Cannot create network_dir: %s", c.NetworkDir)
				}
			} else {
				return errors.Wrapf(err, "invalid network_dir: %s", c.NetworkDir)
			}
		}

		for _, pluginDir := range c.PluginDirs {
			if err := os.MkdirAll(pluginDir, 0755); err != nil {
				return errors.Wrapf(err, "invalid plugin_dirs entry")
			}
		}
		// While the plugin_dir option is being deprecated, we need this check
		if c.PluginDir != "" {
			logrus.Warnf("The config field plugin_dir is being deprecated. Please use plugin_dirs instead")
			if err := os.MkdirAll(c.PluginDir, 0755); err != nil {
				return errors.Wrapf(err, "invalid plugin_dir entry")
			}
			// Append PluginDir to PluginDirs, so from now on we can operate in terms of PluginDirs and not worry
			// about missing cases.
			c.PluginDirs = append(c.PluginDirs, c.PluginDir)

			// Empty the pluginDir so on future config calls we don't print it out
			// thus seemlessly transitioning and depreciating the option
			c.PluginDir = ""
		}
	}

	return nil
}
