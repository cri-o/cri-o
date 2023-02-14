package nri

import (
	"time"

	nri "github.com/containerd/nri/pkg/adaptation"
)

// Config represents the CRI-O NRI configuration.
type Config struct {
	Enabled                   bool          `toml:"enable_nri"`
	SocketPath                string        `toml:"nri_listen"`
	PluginPath                string        `toml:"nri_plugin_dir"`
	PluginConfigPath          string        `toml:"nri_plugin_config_dir"`
	PluginRegistrationTimeout time.Duration `toml:"nri_plugin_registration_timeout"`
	PluginRequestTimeout      time.Duration `toml:"nri_plugin_request_timeout"`
	DisableConnections        bool          `toml:"nri_disable_connections"`
}

// New returns the default CRI-O NRI configuration.
func New() *Config {
	return &Config{
		SocketPath:                nri.DefaultSocketPath,
		PluginPath:                nri.DefaultPluginPath,
		PluginConfigPath:          nri.DefaultPluginConfigPath,
		PluginRegistrationTimeout: nri.DefaultPluginRegistrationTimeout,
		PluginRequestTimeout:      nri.DefaultPluginRequestTimeout,
	}
}

// Validate loads and validates the effective runtime NRI configuration.
func (c *Config) Validate(onExecution bool) error {
	return nil
}

// ToOptions returns NRI options for this configuration.
func (c *Config) ToOptions() []nri.Option {
	opts := []nri.Option{}
	if c != nil && c.SocketPath != "" {
		opts = append(opts, nri.WithSocketPath(c.SocketPath))
	}
	if c != nil && c.PluginPath != "" {
		opts = append(opts, nri.WithPluginPath(c.PluginPath))
	}
	if c != nil && c.PluginConfigPath != "" {
		opts = append(opts, nri.WithPluginConfigPath(c.PluginConfigPath))
	}
	if c != nil && c.DisableConnections {
		opts = append(opts, nri.WithDisabledExternalConnections())
	}
	return opts
}

func (c *Config) ConfigureTimeouts() {
	if c.PluginRegistrationTimeout != 0 {
		nri.SetPluginRegistrationTimeout(c.PluginRegistrationTimeout)
	}
	if c.PluginRequestTimeout != 0 {
		nri.SetPluginRequestTimeout(c.PluginRequestTimeout)
	}
}
