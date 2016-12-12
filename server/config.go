package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
)

// Config represents the entire set of configuration values that can be set for
// the server. This is intended to be loaded from a toml-encoded config file.
type Config struct {
	RootConfig
	APIConfig
	RuntimeConfig
	ImageConfig
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

// tomlConfig is another way of looking at a Config, which is
// TOML-friendly (it has all of the explicit tables). It's just used for
// conversions.
type tomlConfig struct {
	Ocid struct {
		RootConfig
		API     struct{ APIConfig }     `toml:"api"`
		Runtime struct{ RuntimeConfig } `toml:"runtime"`
		Image   struct{ ImageConfig }   `toml:"image"`
	} `toml:"ocid"`
}

func (t *tomlConfig) toConfig(c *Config) {
	c.RootConfig = t.Ocid.RootConfig
	c.APIConfig = t.Ocid.API.APIConfig
	c.RuntimeConfig = t.Ocid.Runtime.RuntimeConfig
	c.ImageConfig = t.Ocid.Image.ImageConfig
}

func (t *tomlConfig) fromConfig(c *Config) {
	t.Ocid.RootConfig = c.RootConfig
	t.Ocid.API.APIConfig = c.APIConfig
	t.Ocid.Runtime.RuntimeConfig = c.RuntimeConfig
	t.Ocid.Image.ImageConfig = c.ImageConfig
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

// Check resolves that all paths and files are available as expected
func (c *Config) Check() error {
	for _, file := range []string{
		c.RootConfig.SandboxDir,
		c.RootConfig.ContainerDir,
		c.RuntimeConfig.Runtime,
		c.RuntimeConfig.Conmon,
		c.RuntimeConfig.SeccompProfile,
		c.ImageConfig.Pause,
	} {
		if _, err := os.Stat(file); err != nil && os.IsNotExist(err) {
			return checkError{Path: file}
		}
	}

	return nil
}

type checkError struct {
	Path string
}

func (ce checkError) Error() string {
	return fmt.Sprintf("%q does not exist", ce.Path)
}
