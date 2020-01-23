package server

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/containers/image/v5/types"
	"github.com/cri-o/cri-o/lib/config"
	"github.com/cri-o/cri-o/oci"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultGRPCMaxMsgSize is the default message size maximum for grpc APIs.
	DefaultGRPCMaxMsgSize = 16 * 1024 * 1024
)

// Config represents the entire set of configuration values that can be set for
// the server. This is intended to be loaded from a toml-encoded config file.
type Config struct {
	config.Config
	APIConfig
}

// ConfigIface provides a server config interface for data encapsulation
type ConfigIface interface {
	GetData() *Config
	GetLibConfigIface() config.Iface
}

// GetData returns the Config of a ConfigIface
func (c *Config) GetData() *Config {
	return c
}

// GetLibConfigIface returns the library config interface of a ConfigIface
func (c *Config) GetLibConfigIface() config.Iface {
	return c.Config.GetData()
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
	HostIP string `toml:"host_ip"`
}

// tomlConfig is another way of looking at a Config, which is
// TOML-friendly (it has all of the explicit tables). It's just used for
// conversions.
type tomlConfig struct {
	Crio struct {
		config.RootConfig
		API     struct{ APIConfig }            `toml:"api"`
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
// fails, which should never happen at all because of generael type safeness.
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
	conf, err := config.DefaultConfig()
	if err != nil {
		return nil, err
	}
	return &Config{
		Config: *conf,
		APIConfig: APIConfig{
			Listen:             CrioSocketPath,
			StreamAddress:      "127.0.0.1",
			StreamPort:         "0",
			GRPCMaxSendMsgSize: DefaultGRPCMaxMsgSize,
			GRPCMaxRecvMsgSize: DefaultGRPCMaxMsgSize,
		},
	}, nil
}

// Validate is the main entry point for configuration validation
// The parameter `onExecution` specifies if the validation should include
// execution checks. It returns an `error` on validation failure, otherwise
// `nil`.
func (c *Config) Validate(systemContext *types.SystemContext, onExecution bool) error {
	switch c.ImageVolumes {
	case config.ImageVolumesMkdir:
	case config.ImageVolumesIgnore:
	case config.ImageVolumesBind:
	default:
		return fmt.Errorf("unrecognized image volume type specified")
	}

	if err := c.Config.Validate(systemContext, onExecution); err != nil {
		return errors.Wrapf(err, "library config validation")
	}

	if c.UIDMappings != "" && c.ManageNetworkNSLifecycle {
		return fmt.Errorf("cannot use UIDMappings with ManageNetworkNSLifecycle")
	}
	if c.GIDMappings != "" && c.ManageNetworkNSLifecycle {
		return fmt.Errorf("cannot use GIDMappings with ManageNetworkNSLifecycle")
	}

	if c.LogSizeMax >= 0 && c.LogSizeMax < oci.BufSize {
		return fmt.Errorf("log size max should be negative or >= %d", oci.BufSize)
	}

	return nil
}

// Reload reloads the configuration with the config at the provided `fileName`
// path. The method errors in case of any read or update failure.
func (c *Config) Reload(fileName string) error {
	// Reload the config
	newConfig, err := DefaultConfig()
	if err != nil {
		return fmt.Errorf("unable to create default config")
	}
	if err := newConfig.UpdateFromFile(fileName); err != nil {
		return err
	}

	// Reload all available options
	if err := c.ReloadLogLevel(newConfig); err != nil {
		return err
	}
	if err := c.ReloadPauseImage(newConfig); err != nil {
		return err
	}

	return nil
}

// logConfig logs a config set operation as with info verbosity. Please use
// always this function for setting configuration options to ensure consistent
// log outputs
func logConfig(option, value string) {
	logrus.Infof("set config %s to %q", option, value)
}

// ReloadLogLevel updates the LogLevel with the provided `newConfig`. It errors
// if the level is not parsable.
func (c *Config) ReloadLogLevel(newConfig *Config) error {
	if c.LogLevel != newConfig.LogLevel {
		level, err := logrus.ParseLevel(newConfig.LogLevel)
		if err != nil {
			return err
		}
		// Always log this message without considering the current
		logrus.SetLevel(logrus.InfoLevel)
		logConfig("log_level", newConfig.LogLevel)

		logrus.SetLevel(level)
		c.LogLevel = newConfig.LogLevel
	}
	return nil
}

func (c *Config) ReloadPauseImage(newConfig *Config) error {
	if c.PauseImage != newConfig.PauseImage {
		c.PauseImage = newConfig.PauseImage
		logConfig("pause_image", c.PauseImage)
	}
	if c.PauseImageAuthFile != newConfig.PauseImageAuthFile {
		if newConfig.PauseImageAuthFile != "" {
			if _, err := os.Stat(newConfig.PauseImageAuthFile); err != nil {
				return err
			}
		}
		c.PauseImageAuthFile = newConfig.PauseImageAuthFile
		logConfig("pause_image_auth_file", c.PauseImageAuthFile)
	}
	if c.PauseCommand != newConfig.PauseCommand {
		c.PauseCommand = newConfig.PauseCommand
		logConfig("pause_command", c.PauseCommand)
	}
	return nil
}
