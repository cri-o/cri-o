package blockio

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"github.com/intel/goresctrl/pkg/blockio"
)

type Config struct {
	enabled bool
	config  *blockio.Config
}

// New creates a new blockio config instance
func New() *Config {
	c := &Config{
		config: &blockio.Config{},
	}
	return c
}

// Enabled returns true if blockio is enabled in the system
func (c *Config) Enabled() bool {
	return c.enabled
}

// Load loads and validates blockio config
func (c *Config) Load(path string) error {
	c.enabled = false

	if path == "" {
		logrus.Info("No blockio config file specified, blockio not configured")
		return nil
	}

	data, err := os.ReadFile(filepath.Clean(path))
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

	logrus.Infof("Blockio config successfully loaded from %q", path)
	c.config = tmpCfg
	c.enabled = true
	return nil
}
