package lib

import (
	"bytes"
	"io/ioutil"

	"github.com/BurntSushi/toml"
	"github.com/containers/image/pkg/sysregistries"
	"github.com/containers/image/types"
	"github.com/containers/libpod/pkg/hooks"
	"github.com/containers/storage"
	"github.com/kubernetes-sigs/cri-o/oci"
	"github.com/kubernetes-sigs/cri-o/pkg/config"
)

// Defaults if none are specified
const (
	pauseImage          = "k8s.gcr.io/pause:3.1"
	pauseCommand        = "/pause"
	defaultTransport    = "docker://"
	apparmorProfileName = "crio-default"
	cgroupManager       = oci.CgroupfsCgroupsManager
)

// Config represents the entire set of configuration values that can be set for
// the server. This is intended to be loaded from a toml-encoded config file.
type Config struct {
	config.RootConfig
	config.RuntimeConfig
	config.ImageConfig
	config.NetworkConfig
}

const (
	// DefaultPidsLimit is the default value for maximum number of processes
	// allowed inside a container
	DefaultPidsLimit = 1024

	// DefaultLogSizeMax is the default value for the maximum log size
	// allowed for a container. Negative values mean that no limit is imposed.
	DefaultLogSizeMax = -1
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

// tomlConfig is another way of looking at a Config, which is
// TOML-friendly (it has all of the explicit tables). It's just used for
// conversions.
type tomlConfig struct {
	Crio struct {
		config.RootConfig
		Runtime struct{ config.RuntimeConfig } `toml:"runtime"`
		Image   struct{ config.ImageConfig }   `toml:"image"`
		Network struct{ config.NetworkConfig } `toml:"network"`
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
	registries, _ := sysregistries.GetRegistries(&types.SystemContext{})
	insecureRegistries, _ := sysregistries.GetInsecureRegistries(&types.SystemContext{})
	return &Config{
		RootConfig: config.RootConfig{
			Root:            storage.DefaultStoreOptions.GraphRoot,
			RunRoot:         storage.DefaultStoreOptions.RunRoot,
			Storage:         storage.DefaultStoreOptions.GraphDriverName,
			StorageOptions:  storage.DefaultStoreOptions.GraphDriverOptions,
			LogDir:          "/var/log/crio/pods",
			FileLocking:     true,
			FileLockingPath: lockPath,
		},
		RuntimeConfig: config.RuntimeConfig{
			DefaultRuntime: "runc",
			Runtimes: map[string]oci.RuntimeHandler{
				"runc": {
					RuntimePath: "/usr/bin/runc",
				},
			},
			Conmon: conmonPath,
			ConmonEnv: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
			SELinux:                  selinuxEnabled(),
			SeccompProfile:           seccompProfilePath,
			ApparmorProfile:          apparmorProfileName,
			CgroupManager:            cgroupManager,
			PidsLimit:                DefaultPidsLimit,
			ContainerExitsDir:        containerExitsDir,
			ContainerAttachSocketDir: oci.ContainerAttachSocketDir,
			HooksDirPath:             hooks.DefaultDir,
			LogSizeMax:               DefaultLogSizeMax,
			DefaultCapabilities:      DefaultCapabilities,
			LogLevel:                 "error",
			DefaultSysctls:           []string{},
			DefaultUlimits:           []string{},
		},
		ImageConfig: config.ImageConfig{
			DefaultTransport:    defaultTransport,
			PauseImage:          pauseImage,
			PauseCommand:        pauseCommand,
			SignaturePolicyPath: "",
			ImageVolumes:        config.ImageVolumesMkdir,
			Registries:          registries,
			InsecureRegistries:  insecureRegistries,
		},
		NetworkConfig: config.NetworkConfig{
			NetworkDir: cniConfigDir,
			PluginDir:  cniBinDir,
		},
	}
}
