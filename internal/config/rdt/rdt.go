package rdt

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	"github.com/intel/goresctrl/pkg/rdt"
)

const (
	// DefaultRdtConfigFile is the default value for RDT config file path
	DefaultRdtConfigFile = ""
	// ResctrlPrefix is the prefix used for class/closid directories under the resctrl filesystem
	ResctrlPrefix = ""
)

type Config struct {
	supported bool
	enabled   bool
	config    *rdt.Config
}

// New creates a new RDT config instance
func New() *Config {
	c := &Config{
		supported: true,
		config:    &rdt.Config{},
	}

	rdt.SetLogger(logrus.StandardLogger())

	if err := rdt.Initialize(ResctrlPrefix); err != nil {
		c.supported = false
	}
	return c
}

// Supported returns true if RDT is enabled in the host system
func (c *Config) Supported() bool {
	return c.supported
}

// Enabled returns true if RDT is enabled in CRI-O
func (c *Config) Enabled() bool {
	return c.enabled
}

// Load loads and validates RDT config
func (c *Config) Load(path string) error {
	c.enabled = false

	if !c.Supported() {
		logrus.Info("RDT not available in the host system")
		return nil
	}

	if path == "" {
		logrus.Info("No RDT config file specified, RDT not enabled")
		return nil
	}

	tmpCfg, err := loadConfigFile(path)
	if err != nil {
		return err
	}

	if err := rdt.SetConfig(tmpCfg, true); err != nil {
		return fmt.Errorf("configuring RDT failed: %w", err)
	}

	logrus.Infof("RDT enabled, config successfully loaded from %q", path)
	c.enabled = true
	c.config = tmpCfg

	return nil
}

func loadConfigFile(path string) (*rdt.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading rdt config file failed: %w", err)
	}

	c := &rdt.Config{}
	if err = yaml.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parsing RDT config failed: %w", err)
	}

	return c, nil
}

func (c *Config) ContainerClassFromAnnotations(containerName string, containerAnnotations, podAnnotations map[string]string) (string, error) {
	cls, err := rdt.ContainerClassFromAnnotations(containerName, containerAnnotations, podAnnotations)
	if err != nil {
		return "", err
	}
	if cls != "" && !c.Enabled() {
		return "", fmt.Errorf("RDT disabled, refusing to set RDT class of container %q to %q", containerName, cls)
	}
	return cls, nil
}
