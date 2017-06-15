package server

import (
	"bytes"
	"io/ioutil"

	"github.com/BurntSushi/toml"
	"github.com/opencontainers/selinux/go-selinux"
)

// Default paths if none are specified
const (
	crioRoot            = "/var/lib/containers/storage"
	crioRunRoot         = "/var/run/containers/storage"
	conmonPath          = "/usr/local/libexec/crio/conmon"
	pauseImage          = "kubernetes/pause"
	pauseCommand        = "/pause"
	defaultTransport    = "docker://"
	seccompProfilePath  = "/etc/crio/seccomp.json"
	apparmorProfileName = "crio-default"
	cniConfigDir        = "/etc/cni/net.d/"
	cniBinDir           = "/opt/cni/bin/"
	cgroupManager       = "cgroupfs"
)

// Config represents the entire set of configuration values that can be set for
// the server. This is intended to be loaded from a toml-encoded config file.
type Config struct {
	RootConfig
	APIConfig
	RuntimeConfig
	ImageConfig
	NetworkConfig
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

	// LogDir is the default log directory were all logs will go unless kubelet
	// tells us to put them somewhere else.
	LogDir string `toml:"log_dir"`
}

// APIConfig represents the "crio.api" TOML config table.
type APIConfig struct {
	// Listen is the path to the AF_LOCAL socket on which cri-o will listen.
	// This may support proto://addr formats later, but currently this is just
	// a path.
	Listen string `toml:"listen"`

	// StreamAddress is the IP address on which the stream server will listen.
	StreamAddress string `toml:"stream_address"`

	// StreamPort is the port on which the stream server will listen.
	StreamPort string `toml:"stream_port"`
}

// RuntimeConfig represents the "crio.runtime" TOML config table.
type RuntimeConfig struct {
	// Runtime is the OCI compatible runtime used for trusted container workloads.
	// This is a mandatory setting as this runtime will be the default one and
	// will also be used for untrusted container workloads if
	// RuntimeUntrustedWorkload is not set.
	Runtime string `toml:"runtime"`

	// RuntimeUntrustedWorkload is the OCI compatible runtime used for untrusted
	// container workloads. This is an optional setting, except if
	// DefaultWorkloadTrust is set to "untrusted".
	RuntimeUntrustedWorkload string `toml:"runtime_untrusted_workload"`

	// DefaultWorkloadTrust is the default level of trust crio puts in container
	// workloads. This can either be "trusted" or "untrusted" and the default
	// is "trusted"
	// Containers can be run through different container runtimes, depending on
	// the trust hints we receive from kubelet:
	// - If kubelet tags a container workload as untrusted, crio will try first
	// to run it through the untrusted container workload runtime. If it is not
	// set, crio will use the trusted runtime.
	// - If kubelet does not provide any information about the container workload trust
	// level, the selected runtime will depend on the DefaultWorkloadTrust setting.
	// If it is set to "untrusted", then all containers except for the host privileged
	// ones, will be run by the RuntimeUntrustedWorkload runtime. Host privileged
	// containers are by definition trusted and will always use the trusted container
	// runtime. If DefaultWorkloadTrust is set to "trusted", crio will use the trusted
	// container runtime for all containers.
	DefaultWorkloadTrust string `toml:"default_workload_trust"`

	// Conmon is the path to conmon binary, used for managing the runtime.
	Conmon string `toml:"conmon"`

	// ConmonEnv is the environment variable list for conmon process.
	ConmonEnv []string `toml:"conmon_env"`

	// SELinux determines whether or not SELinux is used for pod separation.
	SELinux bool `toml:"selinux"`

	// SeccompProfile is the seccomp json profile path which is used as the
	// default for the runtime.
	SeccompProfile string `toml:"seccomp_profile"`

	// ApparmorProfile is the apparmor profile name which is used as the
	// default for the runtime.
	ApparmorProfile string `toml:"apparmor_profile"`

	// CgroupManager is the manager implementation name which is used to
	// handle cgroups for containers.
	CgroupManager string `toml:"cgroup_manager"`
}

// ImageConfig represents the "crio.image" TOML config table.
type ImageConfig struct {
	// DefaultTransport is a value we prefix to image names that fail to
	// validate source references.
	DefaultTransport string `toml:"default_transport"`
	// PauseImage is the name of an image which we use to instantiate infra
	// containers.
	PauseImage string `toml:"pause_image"`
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
}

// NetworkConfig represents the "crio.network" TOML config table
type NetworkConfig struct {
	// NetworkDir is where CNI network configuration files are stored.
	NetworkDir string `toml:"network_dir"`

	// PluginDir is where CNI plugin binaries are stored.
	PluginDir string `toml:"plugin_dir"`
}

// tomlConfig is another way of looking at a Config, which is
// TOML-friendly (it has all of the explicit tables). It's just used for
// conversions.
type tomlConfig struct {
	Ocid struct {
		RootConfig
		API     struct{ APIConfig }     `toml:"api"`
		Runtime struct{ RuntimeConfig } `toml:"runtime"`
		Image   struct{ ImageConfig }   `toml:"image"`
		Network struct{ NetworkConfig } `toml:"network"`
	} `toml:"crio"`
}

func (t *tomlConfig) toConfig(c *Config) {
	c.RootConfig = t.Ocid.RootConfig
	c.APIConfig = t.Ocid.API.APIConfig
	c.RuntimeConfig = t.Ocid.Runtime.RuntimeConfig
	c.ImageConfig = t.Ocid.Image.ImageConfig
	c.NetworkConfig = t.Ocid.Network.NetworkConfig
}

func (t *tomlConfig) fromConfig(c *Config) {
	t.Ocid.RootConfig = c.RootConfig
	t.Ocid.API.APIConfig = c.APIConfig
	t.Ocid.Runtime.RuntimeConfig = c.RuntimeConfig
	t.Ocid.Image.ImageConfig = c.ImageConfig
	t.Ocid.Network.NetworkConfig = c.NetworkConfig
}

// FromFile populates the Config from the TOML-encoded file at the given path.
// Returns errors encountered when reading or parsing the files, or nil
// otherwise.
func (c *Config) FromFile(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	t := new(tomlConfig)
	t.fromConfig(c)

	_, err = toml.Decode(string(data), t)
	if err != nil {
		return err
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
func DefaultConfig() *Config {
	return &Config{
		RootConfig: RootConfig{
			Root:    crioRoot,
			RunRoot: crioRunRoot,
			LogDir:  "/var/log/crio/pods",
		},
		APIConfig: APIConfig{
			Listen:        "/var/run/crio.sock",
			StreamAddress: "",
			StreamPort:    "10010",
		},
		RuntimeConfig: RuntimeConfig{
			Runtime:                  "/usr/bin/runc",
			RuntimeUntrustedWorkload: "",
			DefaultWorkloadTrust:     "trusted",

			Conmon: conmonPath,
			ConmonEnv: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
			SELinux:         selinux.GetEnabled(),
			SeccompProfile:  seccompProfilePath,
			ApparmorProfile: apparmorProfileName,
			CgroupManager:   cgroupManager,
		},
		ImageConfig: ImageConfig{
			DefaultTransport:    defaultTransport,
			PauseImage:          pauseImage,
			PauseCommand:        pauseCommand,
			SignaturePolicyPath: "",
		},
		NetworkConfig: NetworkConfig{
			NetworkDir: cniConfigDir,
			PluginDir:  cniBinDir,
		},
	}
}
