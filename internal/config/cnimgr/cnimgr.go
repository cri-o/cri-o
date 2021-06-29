package cnimgr

import (
	"sync"
	"time"

	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/wait"
)

type CNIManager struct {
	// cniPlugin is the internal OCI CNI plugin
	plugin    ocicni.CNIPlugin
	lastError error
	watchers  []chan struct{}
	sync.RWMutex
}

func New(defaultNetwork, networkDir string, pluginDirs ...string) (*CNIManager, error) {
	// Init CNI plugin
	plugin, err := ocicni.InitCNI(
		defaultNetwork, networkDir, pluginDirs...,
	)
	if err != nil {
		return nil, errors.Wrap(err, "initialize CNI plugin")
	}
	mgr := &CNIManager{
		plugin: plugin,
	}
	go mgr.pollUntilReady()
	return mgr, nil
}

func (c *CNIManager) pollUntilReady() {
	// nolint:errcheck
	_ = wait.PollInfinite(500*time.Millisecond, func() (bool, error) {
		c.Lock()
		defer c.Unlock()
		if err := c.plugin.Status(); err != nil {
			c.lastError = err
			return false, nil
		}
		c.lastError = nil
		for _, watcher := range c.watchers {
			watcher <- struct{}{}
		}
		return true, nil
	})
}

func (c *CNIManager) ReadyOrError() error {
	c.RLock()
	defer c.RUnlock()
	return c.lastError
}

func (c *CNIManager) Plugin() ocicni.CNIPlugin {
	return c.plugin
}

func (c *CNIManager) AddWatcher() chan struct{} {
	c.Lock()
	defer c.Unlock()
	watcher := make(chan struct{}, 1)
	c.watchers = append(c.watchers, watcher)

	return watcher
}
