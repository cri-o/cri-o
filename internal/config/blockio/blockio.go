package blockio

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/intel/goresctrl/pkg/blockio"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

type Config struct {
	enabled bool
	reload  bool
	path    string
	config  *blockio.Config
}

// New creates a new blockio config instance.
func New() *Config {
	c := &Config{
		config: &blockio.Config{},
	}

	return c
}

// Enabled returns true if blockio is enabled in the system.
func (c *Config) Enabled() bool {
	return c.enabled
}

// SetReload sets the blockio reload option.
func (c *Config) SetReload(reload bool) {
	c.reload = reload
}

// ReloadRequired returns true if reloading configuration and
// rescanning devices is required.
func (c *Config) ReloadRequired() bool {
	return c.reload
}

// Reload (re-)reads the configuration file and rescans block devices in the system.
func (c *Config) Reload() error {
	if c.path == "" {
		return nil
	}

	data, err := os.ReadFile(c.path)
	if err != nil {
		return fmt.Errorf("reading blockio config file failed: %w", err)
	}

	tmpCfg := &blockio.Config{}
	if err = yaml.Unmarshal(data, &tmpCfg); err != nil {
		return fmt.Errorf("parsing blockio config failed: %w", err)
	}

	if err := blockio.SetConfig(tmpCfg, true); err != nil {
		return fmt.Errorf("configuring blockio failed: %w", err)
	}

	c.config = tmpCfg

	return nil
}

// Load loads and validates blockio config.
func (c *Config) Load(path string) error {
	c.enabled = false
	c.path = ""

	if path == "" {
		logrus.Info("No blockio config file specified, blockio not configured")

		return nil
	}

	c.path = filepath.Clean(path)
	if err := c.Reload(); err != nil {
		return err
	}

	logrus.Infof("Blockio config successfully loaded from %q", path)

	c.enabled = true

	return nil
}
