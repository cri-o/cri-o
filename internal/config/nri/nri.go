package nri

import (
	"fmt"

	nri "github.com/containerd/nri/pkg/adaptation"
)

// Config represents the CRI-O NRI configuration.
type Config struct {
	Enabled    bool   `toml:"enable_nri"`
	ConfigPath string `toml:"nri_config_file"`
	SocketPath string `toml:"nri_listen"`
	PluginPath string `toml:"nri_plugin_dir"`
}

// New returns the default CRI-O NRI configuration.
func New() *Config {
	return &Config{
		ConfigPath: nri.DefaultConfigPath,
		SocketPath: nri.DefaultSocketPath,
		PluginPath: nri.DefaultPluginPath,
	}
}

// Validate loads and validates the effective runtime NRI configuration.
func (c *Config) Validate(onExecution bool) error {
	if c.Enabled {
		_, err := nri.ReadConfig(c.ConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load %q: %w", c.ConfigPath, err)
		}
	}

	return nil
}

// ToOptions returns NRI options for this configuration.
func (c *Config) ToOptions() []nri.Option {
	opts := []nri.Option{}
	if c != nil && c.ConfigPath != "" {
		opts = append(opts, nri.WithConfigPath(c.ConfigPath))
	}
	if c != nil && c.SocketPath != "" {
		opts = append(opts, nri.WithSocketPath(c.SocketPath))
	}
	if c != nil && c.PluginPath != "" {
		opts = append(opts, nri.WithPluginPath(c.PluginPath))
	}
	return opts
}
