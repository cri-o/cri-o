package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	conmonconfig "github.com/containers/conmon/runner/config"
	"github.com/containers/image/v5/pkg/sysregistriesv2"
	"github.com/containers/image/v5/types"
	"github.com/containers/libpod/pkg/rootless"
	createconfig "github.com/containers/libpod/pkg/spec"
	"github.com/containers/storage"
	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/version"
	"github.com/cri-o/cri-o/utils"
	units "github.com/docker/go-units"
	selinux "github.com/opencontainers/selinux/go-selinux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Defaults if none are specified
const (
	pauseImage             = "k8s.gcr.io/pause:3.1"
	pauseCommand           = "/pause"
	defaultTransport       = "docker://"
	defaultRuntime         = "runc"
	DefaultRuntimeType     = "oci"
	DefaultRuntimeRoot     = "/run/runc"
	cgroupManager          = "cgroupfs"
	DefaultApparmorProfile = "crio-default-" + version.Version
	defaultGRPCMaxMsgSize  = 16 * 1024 * 1024
	OCIBufSize             = 8192
	RuntimeTypeVM          = "vm"
)

// Config represents the entire set of configuration values that can be set for
// the server. This is intended to be loaded from a toml-encoded config file.
type Config struct {
	RootConfig
	APIConfig
	RuntimeConfig
	ImageConfig
	NetworkConfig
	MetricsConfig
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

	// VersionFile is the location CRI-O will lay down the version file
	VersionFile string `toml:"version_file"`
}

// RuntimeHandler represents each item of the "crio.runtime.runtimes" TOML
// config table.
type RuntimeHandler struct {
	RuntimePath string `toml:"runtime_path"`
	RuntimeType string `toml:"runtime_type"`
	RuntimeRoot string `toml:"runtime_root"`
}

// Multiple runtime Handlers in a map
type Runtimes map[string]*RuntimeHandler

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

	// SeccompProfile is the seccomp.json profile path which is used as the
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

	// LogFilter specifies a regular expression to filter the log messages
	LogFilter string `toml:"log_filter"`

	// Runtimes defines a list of OCI compatible runtimes. The runtime to
	// use is picked based on the runtime_handler provided by the CRI. If
	// no runtime_handler is provided, the runtime will be picked based on
	// the level of trust of the workload.
	Runtimes Runtimes `toml:"runtimes"`

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
	// to the kubernetes log file
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
	// GlobalAuthFile is a path to a file like /var/lib/kubelet/config.json
	// containing credentials necessary for pulling images from secure
	// registries.
	GlobalAuthFile string `toml:"global_auth_file"`
	// PauseImage is the name of an image which we use to instantiate infra
	// containers.
	PauseImage string `toml:"pause_image"`
	// PauseImageAuthFile, if not empty, is a path to a file like
	// /var/lib/kubelet/config.json containing credentials necessary
	// for pulling PauseImage
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

// APIConfig represents the "crio.api" TOML config table.
type APIConfig struct {
	// GRPCMaxSendMsgSize is the maximum grpc send message size in bytes.
	GRPCMaxSendMsgSize int `toml:"grpc_max_send_msg_size"`

	// GRPCMaxRecvMsgSize is the maximum grpc receive message size in bytes.
	GRPCMaxRecvMsgSize int `toml:"grpc_max_recv_msg_size"`

	// Listen is the path to the AF_LOCAL socket on which cri-o will listen.
	// This may support proto://addr formats later, but currently this is just
	// a path.
	Listen string `toml:"listen"`

	// StreamAddress is the IP address on which the stream server will listen.
	StreamAddress string `toml:"stream_address"`

	// StreamPort is the port on which the stream server will listen.
	StreamPort string `toml:"stream_port"`

	// StreamEnableTLS enables encrypted tls transport of the stream server
	StreamEnableTLS bool `toml:"stream_enable_tls"`

	// StreamTLSCert is the x509 certificate file path used to serve the encrypted stream
	StreamTLSCert string `toml:"stream_tls_cert"`

	// StreamTLSKey is the key file path used to serve the encrypted stream
	StreamTLSKey string `toml:"stream_tls_key"`

	// StreamTLSCA is the x509 CA(s) file used to verify and authenticate client
	// communication with the tls encrypted stream
	StreamTLSCA string `toml:"stream_tls_ca"`

	// HostIP is the IP address that the server uses where it needs to use the primary host IP.
	HostIP []string `toml:"host_ip"`
}

// MetricsConfig specifies all necessary configuration for Prometheus based
// metrics retrieval
type MetricsConfig struct {
	// EnableMetrics can be used to globally enable or disable metrics support
	EnableMetrics bool `toml:"enable_metrics"`

	// MetricsPort is the port on which the metrics server will listen.
	MetricsPort int `toml:"metrics_port"`
}

// tomlConfig is another way of looking at a Config, which is
// TOML-friendly (it has all of the explicit tables). It's just used for
// conversions.
type tomlConfig struct {
	Crio struct {
		RootConfig
		API     struct{ APIConfig }     `toml:"api"`
		Runtime struct{ RuntimeConfig } `toml:"runtime"`
		Image   struct{ ImageConfig }   `toml:"image"`
		Network struct{ NetworkConfig } `toml:"network"`
		Metrics struct{ MetricsConfig } `toml:"metrics"`
	} `toml:"crio"`
}

func (t *tomlConfig) toConfig(c *Config) {
	c.RootConfig = t.Crio.RootConfig
	c.APIConfig = t.Crio.API.APIConfig
	c.RuntimeConfig = t.Crio.Runtime.RuntimeConfig
	c.ImageConfig = t.Crio.Image.ImageConfig
	c.NetworkConfig = t.Crio.Network.NetworkConfig
	c.MetricsConfig = t.Crio.Metrics.MetricsConfig
}

func (t *tomlConfig) fromConfig(c *Config) {
	t.Crio.RootConfig = c.RootConfig
	t.Crio.API.APIConfig = c.APIConfig
	t.Crio.Runtime.RuntimeConfig = c.RuntimeConfig
	t.Crio.Image.ImageConfig = c.ImageConfig
	t.Crio.Network.NetworkConfig = c.NetworkConfig
	t.Crio.Metrics.MetricsConfig = c.MetricsConfig
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
	b, err := c.ToBytes()
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, b, 0644)
}

// ToBytes encodes the config into a byte slice. It errors if the encoding
// fails, which should never happen at all because of general type safeness.
func (c *Config) ToBytes() ([]byte, error) {
	var buffer bytes.Buffer
	e := toml.NewEncoder(&buffer)

	tc := tomlConfig{}
	tc.fromConfig(c)

	if err := e.Encode(tc); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// DefaultConfig returns the default configuration for crio.
func DefaultConfig() (*Config, error) {
	storeOpts, err := storage.DefaultStoreOptions(rootless.IsRootless(), rootless.GetRootlessUID())
	if err != nil {
		return nil, err
	}
	return &Config{
		RootConfig: RootConfig{
			Root:           storeOpts.GraphRoot,
			RunRoot:        storeOpts.RunRoot,
			Storage:        storeOpts.GraphDriverName,
			StorageOptions: storeOpts.GraphDriverOptions,
			LogDir:         "/var/log/crio/pods",
			VersionFile:    CrioVersionPath,
		},
		APIConfig: APIConfig{
			Listen:             CrioSocketPath,
			StreamAddress:      "127.0.0.1",
			StreamPort:         "0",
			GRPCMaxSendMsgSize: defaultGRPCMaxMsgSize,
			GRPCMaxRecvMsgSize: defaultGRPCMaxMsgSize,
		},
		RuntimeConfig: RuntimeConfig{
			DefaultRuntime: defaultRuntime,
			Runtimes: Runtimes{
				defaultRuntime: {
					RuntimePath: "",
					RuntimeType: DefaultRuntimeType,
					RuntimeRoot: DefaultRuntimeRoot,
				},
			},
			Conmon: "",
			ConmonEnv: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
			ConmonCgroup:             "pod",
			SELinux:                  selinuxEnabled(),
			SeccompProfile:           "",
			ApparmorProfile:          DefaultApparmorProfile,
			CgroupManager:            cgroupManager,
			DefaultMountsFile:        "",
			PidsLimit:                DefaultPidsLimit,
			ContainerExitsDir:        containerExitsDir,
			ContainerAttachSocketDir: conmonconfig.ContainerAttachSocketDir,
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
			GlobalAuthFile:      "",
			PauseImage:          pauseImage,
			PauseImageAuthFile:  "",
			PauseCommand:        pauseCommand,
			SignaturePolicyPath: "",
			ImageVolumes:        ImageVolumesMkdir,
			Registries:          []string{},
			InsecureRegistries:  []string{},
		},
		NetworkConfig: NetworkConfig{
			NetworkDir: cniConfigDir,
			PluginDirs: []string{cniBinDir},
		},
		MetricsConfig: MetricsConfig{
			EnableMetrics: false,
			MetricsPort:   9090,
		},
	}, nil
}

// Validate is the main entry point for library configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *Config) Validate(systemContext *types.SystemContext, onExecution bool) error {
	switch c.ImageVolumes {
	case ImageVolumesMkdir:
	case ImageVolumesIgnore:
	case ImageVolumesBind:
	default:
		return fmt.Errorf("unrecognized image volume type specified")
	}

	if err := c.RootConfig.Validate(onExecution); err != nil {
		return errors.Wrapf(err, "root config")
	}

	if err := c.RuntimeConfig.Validate(systemContext, onExecution); err != nil {
		return errors.Wrapf(err, "runtime config")
	}

	if err := c.NetworkConfig.Validate(onExecution); err != nil {
		return errors.Wrapf(err, "network config")
	}

	if err := c.APIConfig.Validate(onExecution); err != nil {
		return errors.Wrapf(err, "api config")
	}

	if !c.SELinux {
		selinux.SetDisabled()
	}

	return nil
}

// Validate is the main entry point for API configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *APIConfig) Validate(onExecution bool) error {
	if c.GRPCMaxSendMsgSize <= 0 {
		c.GRPCMaxSendMsgSize = defaultGRPCMaxMsgSize
	}
	if c.GRPCMaxRecvMsgSize <= 0 {
		c.GRPCMaxRecvMsgSize = defaultGRPCMaxMsgSize
	}

	if onExecution {
		if err := os.MkdirAll(filepath.Dir(c.Listen), 0755); err != nil {
			return err
		}

		// Remove the socket if it already exists
		if _, err := os.Stat(c.Listen); err == nil {
			if err := os.Remove(c.Listen); err != nil {
				return err
			}
		}

		// Validate user provided host IPs
		if len(c.HostIP) > 2 {
			return errors.New("It's not possible to assign more than two host IPs")
		}
		for _, ip := range c.HostIP {
			if net.ParseIP(ip) == nil {
				return errors.Errorf("Unable to parse host IP: %v", ip)
			}
		}
	}

	return nil
}

// Validate is the main entry point for root configuration validation.
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *RootConfig) Validate(onExecution bool) error {
	if onExecution {
		if !filepath.IsAbs(c.LogDir) {
			return errors.New("log_dir is not an absolute path")
		}
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
				c.Runtimes[defaultRuntime] = &RuntimeHandler{
					RuntimePath: "",
					RuntimeType: DefaultRuntimeType,
					RuntimeRoot: DefaultRuntimeRoot,
				}
			}
			// Set the DefaultRuntime to runc so we don't fail further along in the code
			c.DefaultRuntime = defaultRuntime
		} else {
			return fmt.Errorf("default_runtime set to %q, but no runtime entry was found for it", c.DefaultRuntime)
		}
	}

	if !(c.ConmonCgroup == "pod" || strings.HasSuffix(c.ConmonCgroup, ".slice")) {
		return errors.New("conmon cgroup should be 'pod' or a systemd slice")
	}

	if c.UIDMappings != "" && c.ManageNetworkNSLifecycle {
		return fmt.Errorf("cannot use UIDMappings with ManageNetworkNSLifecycle")
	}
	if c.GIDMappings != "" && c.ManageNetworkNSLifecycle {
		return fmt.Errorf("cannot use GIDMappings with ManageNetworkNSLifecycle")
	}

	if c.LogSizeMax >= 0 && c.LogSizeMax < OCIBufSize {
		return fmt.Errorf("log size max should be negative or >= %d", OCIBufSize)
	}

	// check for validation on execution
	if onExecution {
		if err := c.ValidateRuntimes(); err != nil {
			return errors.Wrapf(err, "runtime validation")
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

		// Validate the conmon path
		if err := c.ValidateConmonPath("conmon"); err != nil {
			return errors.Wrapf(err, "conmon validation")
		}
	}

	return nil
}

// ValidateRuntimes checks every runtime if its members are valid
func (c *RuntimeConfig) ValidateRuntimes() error {
	// Validate if runtime_path does exist for each runtime
	for name, handler := range c.Runtimes {
		if err := handler.Validate(name); err != nil {
			return err
		}
	}
	return nil
}

// ValidateConmonPath checks if `Conmon` is set within the `RuntimeConfig`.
// If this is not the case, it tries to find it within the $PATH variable.
// In any other case, it simply checks if `Conmon` is a valid file.
func (c *RuntimeConfig) ValidateConmonPath(executable string) error {
	if c.Conmon == "" {
		conmon, err := exec.LookPath(executable)
		if err != nil {
			return err
		}
		c.Conmon = conmon
		logrus.Debugf("using conmon from $PATH")
	} else if _, err := os.Stat(c.Conmon); err != nil {
		return errors.Wrapf(err, "invalid conmon path")
	}
	logrus.Infof("using conmon executable %q", c.Conmon)
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
			// thus seamlessly transitioning and depreciating the option
			c.PluginDir = ""
		}
	}

	return nil
}

// Validate checks if the whole runtime is valid.
func (r *RuntimeHandler) Validate(name string) error {
	if err := r.ValidateRuntimePath(name); err != nil {
		return err
	}
	return r.ValidateRuntimeType(name)
}

// ValidateRuntimePath checks if the `RuntimePath` is either set or available
// within the $PATH environment. The method fails on any `RuntimePath` lookup
// error.
func (r *RuntimeHandler) ValidateRuntimePath(name string) error {
	if r.RuntimePath == "" {
		executable, err := exec.LookPath(name)
		if err != nil {
			return errors.Wrapf(err, "%q not found in $PATH", name)
		}
		r.RuntimePath = executable
		logrus.Debugf("using runtime executable from $PATH %q", executable)
	} else if _, err := os.Stat(r.RuntimePath); os.IsNotExist(err) {
		return fmt.Errorf("invalid runtime_path for runtime '%s': %q",
			name, err)
	}
	logrus.Debugf("found valid runtime %q for runtime_path %q",
		name, r.RuntimePath)
	return nil
}

// ValidateRuntimeType checks if the `RuntimeType` is valid.
func (r *RuntimeHandler) ValidateRuntimeType(name string) error {
	if r.RuntimeType != "" && r.RuntimeType != DefaultRuntimeType && r.RuntimeType != RuntimeTypeVM {
		return errors.Errorf("invalid `runtime_type` %q for runtime %q",
			r.RuntimeType, name)
	}
	return nil
}
