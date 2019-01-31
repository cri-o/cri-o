package server

import (
	"bytes"
	"io/ioutil"

	"github.com/BurntSushi/toml"
	"github.com/kubernetes-sigs/cri-o/lib"
	"github.com/kubernetes-sigs/cri-o/pkg/config"
)

const (
	// DefaultGRPCMaxMsgSize is the default message size maximum for grpc APIs.
	DefaultGRPCMaxMsgSize = 16 * 1024 * 1024
)

// Config represents the entire set of configuration values that can be set for
// the server. This is intended to be loaded from a toml-encoded config file.
type Config struct {
	lib.Config
	config.APIConfig
}

// tomlConfig is another way of looking at a Config, which is
// TOML-friendly (it has all of the explicit tables). It's just used for
// conversions.
type tomlConfig struct {
	Crio struct {
		config.RootConfig
		API     struct{ config.APIConfig }     `toml:"api"`
		Runtime struct{ config.RuntimeConfig } `toml:"runtime"`
		Image   struct{ config.ImageConfig }   `toml:"image"`
		Network struct{ config.NetworkConfig } `toml:"network"`
	} `toml:"crio"`
}

func (t *tomlConfig) toConfig(c *Config) {
	c.RootConfig = t.Crio.RootConfig
	c.APIConfig = t.Crio.API.APIConfig
	c.RuntimeConfig = t.Crio.Runtime.RuntimeConfig
	c.ImageConfig = t.Crio.Image.ImageConfig
	c.NetworkConfig = t.Crio.Network.NetworkConfig
}

func (t *tomlConfig) fromConfig(c *Config) {
	t.Crio.RootConfig = c.RootConfig
	t.Crio.API.APIConfig = c.APIConfig
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
	return &Config{
		Config: *lib.DefaultConfig(),
		APIConfig: config.APIConfig{
			Listen:             CrioSocketPath,
			StreamAddress:      "127.0.0.1",
			StreamPort:         "0",
			GRPCMaxSendMsgSize: DefaultGRPCMaxMsgSize,
			GRPCMaxRecvMsgSize: DefaultGRPCMaxMsgSize,
		},
	}
}
