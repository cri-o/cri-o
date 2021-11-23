//go:build test
// +build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package cnimgr

import (
	"github.com/cri-o/ocicni/pkg/ocicni"
)

// SetCNIPlugin sets the network plugin for the Configuration. The function
// errors if a sane shutdown of the initially created network plugin failed.
func (c *CNIManager) SetCNIPlugin(plugin ocicni.CNIPlugin) error {
	if c.plugin != nil {
		if err := c.plugin.Shutdown(); err != nil {
			return err
		}
	}
	c.plugin = plugin
	// initialize the poll, but don't run it continuously (or else the mocks will get weird)
	//nolint:errcheck
	_, _ = c.pollFunc()
	return nil
}
