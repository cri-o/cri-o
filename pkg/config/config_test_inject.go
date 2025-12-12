//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package config

import (
	"github.com/cri-o/ocicni/pkg/ocicni"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/config/cnimgr"
	"github.com/cri-o/cri-o/internal/config/nsmgr"
)

// SetCNIPlugin sets the network plugin for the Configuration. The function
// errors if a sane shutdown of the initially created network plugin failed.
func (c *Config) SetCNIPlugin(plugin ocicni.CNIPlugin) error {
	if c.cniManager == nil {
		c.cniManager = &cnimgr.CNIManager{}
	}

	return c.cniManager.SetCNIPlugin(plugin)
}

// SetNamespaceManager sets the namespaceManager for the Configuration.
func (c *Config) SetNamespaceManager(nsMgr *nsmgr.NamespaceManager) {
	c.namespaceManager = nsMgr
}

// SetCheckpointRestore offers the possibility to turn on and
// turn off CheckpointRestore support for testing.
func (c *RuntimeConfig) SetCheckpointRestore(cr bool) {
	c.EnableCriuSupport = cr
}

// SetCgroupManager sets the cgroupManager for the RuntimeConfig.
func (c *RuntimeConfig) SetCgroupManager(mgr cgmgr.CgroupManager) {
	c.cgroupManager = mgr
}
