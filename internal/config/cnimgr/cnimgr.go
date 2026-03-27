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

var errShutdown = errors.New("CNI manager shut down")

type CNIManager struct {
	// cniPlugin is the internal OCI CNI plugin
	plugin              ocicni.CNIPlugin
	lastError           error
	watchers            []chan bool
	mutex               sync.RWMutex
	cancel              context.CancelFunc
	initPollInterval    time.Duration
	monitorPollInterval time.Duration

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

	ctx, cancel := context.WithCancel(context.Background())

	mgr := &CNIManager{
		plugin:              plugin,
		lastError:           errors.New("plugin status uninitialized"),
		cancel:              cancel,
		initPollInterval:    500 * time.Millisecond,
		monitorPollInterval: 5 * time.Second,
	}

	go mgr.pollContinuously(ctx)

	return mgr, nil
}

func (c *CNIManager) pollContinuously(ctx context.Context) {
	// Phase 1: fast poll until the plugin is ready for the first time.
	// This handles startup synchronization, triggers deferred GC, and notifies
	// watchers that are blocking pod creation.
	//nolint:errcheck // error is intentionally ignored, status is tracked via lastError
	wait.PollUntilContextCancel(ctx, c.initPollInterval, true,
		func(ctx context.Context) (bool, error) {
			return c.statusPollFunc(ctx, true)
		})

	// Phase 2: slow poll to continuously monitor plugin health.
	// If the plugin becomes unhealthy, lastError is set so that
	// ReadyOrError() reports not-ready and kubelet sees NetworkReady=false.
	//nolint:errcheck // error is intentionally ignored, status is tracked via lastError
	wait.PollUntilContextCancel(ctx, c.monitorPollInterval, true,
		func(ctx context.Context) (bool, error) {
			return c.statusPollFunc(ctx, false)
		})
}

func (c *CNIManager) statusPollFunc(ctx context.Context, isStartup bool) (bool, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err := c.plugin.StatusWithContext(ctx); err != nil {
		c.lastError = err

		return false, nil
	}

	if c.lastError != nil {
		c.lastError = nil
		// on startup, GC might have been attempted before the plugin was
		// actually ready so we might have deferred it until now, which is
		// still a good time to do it as the relevant context is equivalent:
		// the same list of pods is valid and stable because new pods can't
		// be created until the plugin is announced as ready
		if isStartup {
			if err := c.doGC(ctx); err != nil {
				logrus.Warnf("Garbage collect stale network resources failed: %v", err)
			}
		}
		// Notify any waiters blocked on pod creation that the plugin is
		// now ready. Non-blocking send avoids deadlock if a watcher was
		// abandoned (e.g. timed-out pod creation) and the buffer is full.
		for _, watcher := range c.watchers {
			select {
			case watcher <- true:
			default:
			}
		}

		c.watchers = nil
	}

	return isStartup, nil
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

	switch {
	case c.lastError == nil:
		watcher <- true
	case errors.Is(c.lastError, errShutdown):
		watcher <- false
	default:
		c.watchers = append(c.watchers, watcher)
	}

	return watcher
}

// Shutdown shuts down the CNI manager, and notifies the watcher
// that the CNI manager is not ready.
func (c *CNIManager) Shutdown() {
	if c.cancel != nil {
		c.cancel()
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.lastError = errShutdown

	// Non-blocking send: the buffer may already contain a `true` from a
	// recent recovery notification. In that case the consumer will read
	// `true` and proceed, which is acceptable since the plugin was ready
	// at that point; the shutdown will be caught by subsequent operations.
	for _, watcher := range c.watchers {
		select {
		case watcher <- false:
		default:
		}
	}

	c.watchers = nil
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
		// so defer until it is (see statusPollFunc)
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
