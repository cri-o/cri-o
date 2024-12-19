package cnimgr

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
)

type PodNetworkLister func() ([]*ocicni.PodNetwork, error)

type CNIManager struct {
	// cniPlugin is the internal OCI CNI plugin
	plugin    ocicni.CNIPlugin
	lastError error
	watchers  []chan bool
	shutdown  bool
	mutex     sync.RWMutex

	validPodList PodNetworkLister
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
		plugin:    plugin,
		lastError: errors.New("plugin status uninitialized"),
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
	// on startup, GC might have been attempted before the plugin was actually
	// ready so we might have deferred it until now, which is still a good time
	// to do it as the relevant context is equivalent: the same list of pods is
	// valid and stable because new pods can't be created until the plugin is
	// announced as ready
	if err := c.doGC(context.Background()); err != nil {
		logrus.Warnf("Garbage collect stale network resources during plugin startup failed: %v", err)
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

// GC calls the plugin's GC to clean up any resources concerned with stale pods
// (pod other than the ones provided by validPodList). The call to the plugin
// will be deferred until it is ready logging any errors then and returning nil
// error here.
func (c *CNIManager) GC(ctx context.Context, validPodList PodNetworkLister) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.validPodList = validPodList
	if c.lastError != nil {
		// on startup, GC might be attempted before the plugin is actually ready
		// so defer until it is (see pollFunc)
		return nil
	}
	return c.doGC(ctx)
}

func (c *CNIManager) doGC(ctx context.Context) error {
	if c.validPodList == nil {
		return nil
	}
	validPods, err := c.validPodList()
	if err != nil {
		return err
	}
	// give a GC call 30s
	stopCtx, stopCancel := context.WithTimeout(ctx, 30*time.Second)
	defer stopCancel()
	return c.plugin.GC(stopCtx, validPods)
}
