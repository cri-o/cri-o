package server

import (
	"bytes"
	"io/ioutil"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/opencontainers/runc/libcontainer/selinux"
)

// Default paths if none are specified
const (
	ocidRoot           = "/var/lib/ocid"
	conmonPath         = "/usr/libexec/ocid/conmon"
	pausePath          = "/usr/libexec/ocid/pause"
	seccompProfilePath = "/etc/ocid/seccomp.json"
	cniConfigDir       = "/etc/cni/net.d/"
	cniBinDir          = "/opt/cni/bin/"
)

const (
	apparmorProfileName = "ocid-default"
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

// RootConfig represents the root of the "ocid" TOML config table.
type RootConfig struct {
	// Root is a path to the "root directory" where all information not
	// explicitly handled by other options will be stored.
	Root string `toml:"root"`

	// SandboxDir is the directory where ocid will store all of its sandbox
	// state and other information.
	SandboxDir string `toml:"sandbox_dir"`

	// ContainerDir is the directory where ocid will store all of its container
	// state and other information.
	ContainerDir string `toml:"container_dir"`

	// LogDir is the default log directory were all logs will go unless kubelet
	// tells us to put them somewhere else.
	//
	// TODO: This is currently unused until the conmon logging rewrite is done.
	LogDir string `toml:"log_dir"`
}

// APIConfig represents the "ocid.api" TOML config table.
type APIConfig struct {
	// Listen is the path to the AF_LOCAL socket on which cri-o will listen.
	// This may support proto://addr formats later, but currently this is just
	// a path.
	Listen string `toml:"listen"`
}

// RuntimeConfig represents the "ocid.runtime" TOML config table.
type RuntimeConfig struct {
	// Runtime is a path to the OCI runtime which ocid will be using. Currently
	// the only known working choice is runC, simply because the OCI has not
	// yet merged a CLI API (so we assume runC's API here).
	Runtime string `toml:"runtime"`

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

// ImageConfig represents the "ocid.image" TOML config table.
type ImageConfig struct {
	// Pause is the path to the statically linked pause container binary, used
	// as the entrypoint for infra containers.
	//
	// TODO(cyphar): This should be replaced with a path to an OCI image
	// bundle, once the OCI image/storage code has been implemented.
	Pause string `toml:"pause"`

	// ImageStore is the directory where the ocid image store will be stored.
	// TODO: This is currently not really used because we don't have
	//       containers/storage integrated.
	ImageDir string `toml:"image_dir"`
}

// NetworkConfig represents the "ocid.network" TOML config table
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
	} `toml:"ocid"`
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

// DefaultConfig returns the default configuration for ocid.
func DefaultConfig() *Config {
	return &Config{
		RootConfig: RootConfig{
			Root:         ocidRoot,
			SandboxDir:   filepath.Join(ocidRoot, "sandboxes"),
			ContainerDir: filepath.Join(ocidRoot, "containers"),
			LogDir:       "/var/log/ocid/pods",
		},
		APIConfig: APIConfig{
			Listen: "/var/run/ocid.sock",
		},
		RuntimeConfig: RuntimeConfig{
			Runtime: "/usr/bin/runc",
			Conmon:  conmonPath,
			ConmonEnv: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
			SELinux:         selinux.SelinuxEnabled(),
			SeccompProfile:  seccompProfilePath,
			ApparmorProfile: apparmorProfileName,
			CgroupManager:   cgroupManager,
		},
		ImageConfig: ImageConfig{
			Pause:    pausePath,
			ImageDir: filepath.Join(ocidRoot, "store"),
		},
		NetworkConfig: NetworkConfig{
			NetworkDir: cniConfigDir,
			PluginDir:  cniBinDir,
		},
	}
}
