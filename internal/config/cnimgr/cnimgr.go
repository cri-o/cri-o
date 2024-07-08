package cnimgr

import (
	"fmt"
	"sync"
	"time"

	"github.com/cri-o/ocicni/pkg/ocicni"
	"k8s.io/apimachinery/pkg/util/wait"
)

type CNIManager struct {
	// cniPlugin is the internal OCI CNI plugin
	plugin    ocicni.CNIPlugin
	lastError error
	watchers  []chan bool
	shutdown  bool
	mutex     sync.RWMutex
}

func New(defaultNetwork, networkDir string, pluginDirs ...string) (*CNIManager, error) {
	// Init CNI plugin
	plugin, err := ocicni.InitCNI(
		defaultNetwork, networkDir, pluginDirs...,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize CNI plugin: %w", err)
	}
	mgr := &CNIManager{
		plugin: plugin,
	}
	go mgr.pollUntilReady()
	return mgr, nil
}

func (c *CNIManager) pollUntilReady() {
	//nolint:errcheck,staticcheck
	_ = wait.PollInfinite(500*time.Millisecond, c.pollFunc)
}

func (c *CNIManager) pollFunc() (bool, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.shutdown {
		return true, nil
	}
	if err := c.plugin.Status(); err != nil {
		c.lastError = err
		return false, nil
	}
	c.lastError = nil
	for _, watcher := range c.watchers {
		watcher <- true
	}
	return true, nil
}

// ReadyOrError returns nil if the plugin is ready,
// or the last error that was received in checking.
func (c *CNIManager) ReadyOrError() error {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.lastError
}

// Plugin returns the CNI plugin.
func (c *CNIManager) Plugin() ocicni.CNIPlugin {
	return c.plugin
}

// Add watcher creates a new watcher for the CNI manager
// said watcher will send a `true` value if the CNI plugin was successfully ready
// or `false` if the server shutdown first.
func (c *CNIManager) AddWatcher() chan bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	watcher := make(chan bool, 1)
	c.watchers = append(c.watchers, watcher)

	return watcher
}

// Shutdown shuts down the CNI manager, and notifies the watcher
// that the CNI manager is not ready.
func (c *CNIManager) Shutdown() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.shutdown = true
	for _, watcher := range c.watchers {
		watcher <- false
	}
}
