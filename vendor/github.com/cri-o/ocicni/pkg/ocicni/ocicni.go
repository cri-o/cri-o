package ocicni

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/containernetworking/cni/libcni"
	cniinvoke "github.com/containernetworking/cni/pkg/invoke"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	cniv1 "github.com/containernetworking/cni/pkg/types/100"
	cniversion "github.com/containernetworking/cni/pkg/version"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
)

type cniNetworkPlugin struct {
	cniConfig *libcni.CNIConfig

	sync.RWMutex
	defaultNetName netName
	networks       map[string]*cniNetwork

	nsManager *nsManager
	confDir   string
	binDirs   []string

	shutdownChan chan struct{}
	watcher      *fsnotify.Watcher
	done         *sync.WaitGroup

	// The pod map provides synchronization for a given pod's network
	// operations.  Each pod's setup/teardown/status operations
	// are synchronized against each other, but network operations of other
	// pods can proceed in parallel.
	podsLock sync.Mutex
	pods     map[string]*podLock

	// The gcLock blocks *all* pod operations from taking place
	// while GC is happening.
	//
	// This must be acquired first, to prevent deadlocks.
	gcLock sync.RWMutex

	// For testcases
	exec     cniinvoke.Exec
	cacheDir string
}

type netName struct {
	name       string
	changeable bool
}

type cniNetwork struct {
	name     string
	filePath string
	config   *libcni.NetworkConfigList
}

const errMissingDefaultNetwork = "no CNI configuration file in %s. Has your network provider started?"

type podLock struct {
	// Count of in-flight operations for this pod; when this reaches zero
	// the lock can be removed from the pod map
	refcount uint

	// Lock to synchronize operations for this specific pod
	mu sync.Mutex
}

func buildFullPodName(podNetwork *PodNetwork) string {
	return podNetwork.Namespace + "_" + podNetwork.Name
}

// Lock network operations for a specific pod.  If that pod is not yet in
// the pod map, it will be added.  The reference count for the pod will
// be increased.
func (plugin *cniNetworkPlugin) podLock(podNetwork *PodNetwork) {
	plugin.podsLock.Lock()

	fullPodName := buildFullPodName(podNetwork)

	lock, ok := plugin.pods[fullPodName]
	if !ok {
		lock = &podLock{}
		plugin.pods[fullPodName] = lock
	}

	lock.refcount++
	plugin.podsLock.Unlock()
	lock.mu.Lock()
}

// Unlock network operations for a specific pod.  The reference count for the
// pod will be decreased.  If the reference count reaches zero, the pod will be
// removed from the pod map.
func (plugin *cniNetworkPlugin) podUnlock(podNetwork *PodNetwork) {
	plugin.podsLock.Lock()
	defer plugin.podsLock.Unlock()

	fullPodName := buildFullPodName(podNetwork)
	lock, ok := plugin.pods[fullPodName]

	if !ok {
		logrus.Errorf("Cannot find reference in refcount map for %s. Refcount cannot be determined.", fullPodName)

		return
	} else if lock.refcount == 0 {
		// This should never ever happen, but handle it anyway
		delete(plugin.pods, fullPodName)
		logrus.Errorf("Pod lock for %s still in map with zero refcount", fullPodName)

		return
	}

	lock.refcount--
	lock.mu.Unlock()

	if lock.refcount == 0 {
		delete(plugin.pods, fullPodName)
	}
}

func newWatcher(dirs []string) (*fsnotify.Watcher, error) {
	// Ensure directories exist because the fsnotify watch logic depends on it
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create directory %q: %w", dir, err)
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create new watcher %w", err)
	}

	defer func() {
		// Close watcher on error
		if err != nil {
			watcher.Close()
		}
	}()

	for _, dir := range dirs {
		if err = watcher.Add(dir); err != nil {
			return nil, fmt.Errorf("failed to add watch on %q: %w", dir, err)
		}
	}

	return watcher, nil
}

func (plugin *cniNetworkPlugin) monitorConfDir(ctx context.Context, start *sync.WaitGroup) {
	start.Done()
	plugin.done.Add(1)

	defer plugin.done.Done()

	exts := []string{".conf", ".conflist", ".json"}

	for {
		select {
		case event := <-plugin.watcher.Events:
			if slices.Contains(exts, filepath.Ext(event.Name)) {
				logrus.Infof("CNI monitoring event %v", event)
			}

			var defaultDeleted bool

			createWriteRename := event.Op&fsnotify.Create == fsnotify.Create ||
				event.Op&fsnotify.Write == fsnotify.Write ||
				event.Op&fsnotify.Rename == fsnotify.Rename

			if event.Op&fsnotify.Remove == fsnotify.Remove {
				// Care about the event if the default network
				// was just deleted
				defNet := plugin.getDefaultNetwork()
				if defNet != nil && event.Name == defNet.filePath {
					defaultDeleted = true
				}
			}

			if !createWriteRename && !defaultDeleted {
				continue
			}

			if err := plugin.syncNetworkConfig(ctx); err != nil {
				logrus.Errorf("CNI config loading failed, continue monitoring: %v", err)

				continue
			}

		case err := <-plugin.watcher.Errors:
			if err == nil {
				continue
			}

			logrus.Errorf("CNI monitoring error %v", err)

			return

		case <-plugin.shutdownChan:
			return
		}
	}
}

// InitCNI takes a binary directory in which to search for CNI plugins, and
// a configuration directory in which to search for CNI JSON config files.
// If no valid CNI configs exist, network requests will fail until valid CNI
// config files are present in the config directory.
// If defaultNetName is not empty, a CNI config with that network name will
// be used as the default CNI network, and container network operations will
// fail until that network config is present and valid.
// If defaultNetName is empty, CNI config files should be reloaded real-time and
// defaultNetName should be changeable and determined by file sorting.
func InitCNI(defaultNetName, confDir string, binDirs ...string) (CNIPlugin, error) {
	return initCNI(nil, "", defaultNetName, confDir, true, binDirs...)
}

// InitCNIWithCache works like InitCNI except that it takes the cni cache directory as third param.
func InitCNIWithCache(defaultNetName, confDir, cacheDir string, binDirs ...string) (CNIPlugin, error) {
	return initCNI(nil, cacheDir, defaultNetName, confDir, true, binDirs...)
}

// InitCNINoInotify works like InitCNI except that it does not use inotify to watch for changes in the CNI config dir.
func InitCNINoInotify(defaultNetName, confDir, cacheDir string, binDirs ...string) (CNIPlugin, error) {
	return initCNI(nil, cacheDir, defaultNetName, confDir, false, binDirs...)
}

// Internal function to allow faking out exec functions for testing.
func initCNI(exec cniinvoke.Exec, cacheDir, defaultNetName, confDir string, useInotify bool, binDirs ...string) (CNIPlugin, error) {
	if confDir == "" {
		confDir = DefaultConfDir
	}

	if len(binDirs) == 0 {
		binDirs = []string{DefaultBinDir}
	}

	if exec == nil {
		exec = &cniinvoke.DefaultExec{
			RawExec:       &cniinvoke.RawExec{Stderr: os.Stderr},
			PluginDecoder: cniversion.PluginDecoder{},
		}
	}

	plugin := &cniNetworkPlugin{
		cniConfig: libcni.NewCNIConfigWithCacheDir(binDirs, cacheDir, exec),
		defaultNetName: netName{
			name: defaultNetName,
			// If defaultNetName is not assigned in initialization,
			// it should be changeable
			changeable: defaultNetName == "",
		},
		networks:     make(map[string]*cniNetwork),
		confDir:      confDir,
		binDirs:      binDirs,
		shutdownChan: make(chan struct{}),
		done:         &sync.WaitGroup{},
		pods:         make(map[string]*podLock),
		exec:         exec,
		cacheDir:     cacheDir,
	}

	nsm, err := newNSManager()
	if err != nil {
		return nil, err
	}

	plugin.nsManager = nsm

	ctx := context.Background()
	if err := plugin.syncNetworkConfig(ctx); err != nil {
		logrus.Errorf("CNI sync network config failed: %v", err)
	}

	if useInotify {
		plugin.watcher, err = newWatcher(append([]string{plugin.confDir}, binDirs...))
		if err != nil {
			return nil, err
		}

		startWg := sync.WaitGroup{}
		startWg.Add(1)

		go plugin.monitorConfDir(ctx, &startWg)
		startWg.Wait()
	}

	return plugin, nil
}

func (plugin *cniNetworkPlugin) Shutdown() error {
	close(plugin.shutdownChan)

	if plugin.watcher != nil {
		plugin.watcher.Close()
	}

	plugin.done.Wait()

	return nil
}

func loadNetworks(ctx context.Context, confDir string, cni *libcni.CNIConfig) (networks map[string]*cniNetwork, defaultNetName string, err error) {
	files, err := libcni.ConfFiles(confDir, []string{".conf", ".conflist", ".json"})
	if err != nil {
		return nil, "", err
	}

	networks = make(map[string]*cniNetwork)

	sort.Strings(files)

	for _, confFile := range files {
		var confList *libcni.NetworkConfigList
		if strings.HasSuffix(confFile, ".conflist") {
			confList, err = libcni.ConfListFromFile(confFile)
			if err != nil {
				// do not log ENOENT errors
				if !os.IsNotExist(err) {
					logrus.Errorf("Error loading CNI config list file %s: %v", confFile, err)
				}

				continue
			}
		} else {
			bytes, err := os.ReadFile(confFile)
			if err != nil {
				logrus.Errorf("Error loading CNI config file %s: %v", confFile, err)

				continue
			}

			conf, err := libcni.NetworkPluginConfFromBytes(bytes)
			if err != nil {
				// do not log ENOENT errors
				if !os.IsNotExist(err) {
					logrus.Errorf("Error loading CNI config file %s: %v", confFile, err)
				}

				continue
			}

			//nolint:staticcheck // we still require this function
			confList, err = libcni.ConfListFromConf(conf)
			if err != nil {
				logrus.Errorf("Error converting CNI config file %s to list: %v", confFile, err)

				continue
			}
		}

		if len(confList.Plugins) == 0 {
			logrus.Infof("CNI config list %s has no networks, skipping", confFile)

			continue
		}

		// Validation on CNI config should be done to pre-check presence
		// of plugins which are necessary.
		if _, err := cni.ValidateNetworkList(ctx, confList); err != nil {
			logrus.Warningf("Error validating CNI config file %s: %v", confFile, err)

			continue
		}

		if confList.Name == "" {
			confList.Name = path.Base(confFile)
		}

		cniNet := &cniNetwork{
			name:     confList.Name,
			filePath: confFile,
			config:   confList,
		}

		logrus.Infof("Found CNI network %s (type=%v) at %s", confList.Name, confList.Plugins[0].Network.Type, confFile)

		if _, ok := networks[confList.Name]; !ok {
			networks[confList.Name] = cniNet
		} else {
			logrus.Infof("Ignored CNI network %s (type=%v) at %s because already exists", confList.Name, confList.Plugins[0].Network.Type, confFile)
		}

		if defaultNetName == "" {
			defaultNetName = confList.Name
		}
	}

	return networks, defaultNetName, nil
}

const (
	loIfname string = "lo"
)

func (plugin *cniNetworkPlugin) syncNetworkConfig(ctx context.Context) error {
	networks, defaultNetName, err := loadNetworks(ctx, plugin.confDir, plugin.cniConfig)
	if err != nil {
		return err
	}

	plugin.Lock()
	defer plugin.Unlock()

	// Update defaultNetName if it is changeable
	if plugin.defaultNetName.changeable {
		plugin.defaultNetName.name = defaultNetName

		if defaultNetName != "" {
			logrus.Infof("Updated default CNI network name to %s", defaultNetName)
		}
	} else {
		logrus.Debugf("Default CNI network name %s is unchangeable", plugin.defaultNetName.name)
	}

	plugin.networks = networks

	return nil
}

func (plugin *cniNetworkPlugin) GetDefaultNetworkName() string {
	plugin.RLock()
	defer plugin.RUnlock()

	return plugin.defaultNetName.name
}

func (plugin *cniNetworkPlugin) getDefaultNetwork() *cniNetwork {
	plugin.RLock()
	defer plugin.RUnlock()

	defaultNetName := plugin.defaultNetName.name
	if defaultNetName == "" {
		return nil
	}

	network, ok := plugin.networks[defaultNetName]
	if !ok {
		logrus.Debugf("Failed to get network for name: %s", defaultNetName)
	}

	return network
}

// networksAvailable returns an error if the pod requests no networks and the
// plugin has no default network, and thus the plugin has no idea what network
// to attach the pod to.
func (plugin *cniNetworkPlugin) networksAvailable(podNetwork *PodNetwork) error {
	if len(podNetwork.Networks) == 0 && plugin.getDefaultNetwork() == nil {
		return fmt.Errorf(errMissingDefaultNetwork, plugin.confDir)
	}

	return nil
}

func (plugin *cniNetworkPlugin) Name() string {
	return CNIPluginName
}

func (plugin *cniNetworkPlugin) loadNetworkFromCache(name string, rt *libcni.RuntimeConf) (*cniNetwork, *libcni.RuntimeConf, error) {
	cniNet := &cniNetwork{
		name: name,
		config: &libcni.NetworkConfigList{
			Name: name,
		},
	}

	var confBytes []byte

	var err error

	confBytes, rt, err = plugin.cniConfig.GetNetworkListCachedConfig(cniNet.config, rt)
	if err != nil {
		return nil, nil, err
	} else if confBytes == nil {
		return nil, nil, fmt.Errorf("network %q not found in CNI cache", name)
	}

	cniNet.config, err = libcni.NetworkConfFromBytes(confBytes)
	if err != nil {
		return nil, nil, err
	}

	if len(cniNet.config.Plugins) == 0 {
		// Might be a plain NetworkConfig
		netConf, err := libcni.NetworkPluginConfFromBytes(confBytes)
		if err != nil {
			return nil, nil, err
		}

		cniNet.config.Plugins = []*libcni.PluginConfig{netConf}
	}

	return cniNet, rt, nil
}

// fillPodNetworks inserts any needed values in the set of pod network requests:
// - if no networks, add default
// - if no interface names, synthesize
//
// plugin RLock must be held.
func (plugin *cniNetworkPlugin) fillPodNetworks(podNetwork *PodNetwork) error {
	if len(podNetwork.Networks) == 0 {
		podNetwork.Networks = append(podNetwork.Networks, NetAttachment{
			Name: plugin.defaultNetName.name,
		})
	}

	allIfNames := make(map[string]bool)

	for _, net := range podNetwork.Networks {
		if net.Ifname != "" {
			// Make sure the requested name isn't already assigned
			if allIfNames[net.Ifname] {
				return fmt.Errorf("network %q requested interface name %q already assigned", net.Name, net.Ifname)
			}

			allIfNames[net.Ifname] = true
		}
	}
netLoop:
	for i, network := range podNetwork.Networks {
		if network.Ifname == "" {
			for j := range 10000 {
				candidate := fmt.Sprintf("eth%d", j)
				if !allIfNames[candidate] {
					allIfNames[candidate] = true
					podNetwork.Networks[i].Ifname = candidate

					continue netLoop
				}
			}

			return fmt.Errorf("failed to find free interface name for network %q", network.Name)
		}
	}

	return nil
}

type forEachNetworkFn func(*cniNetwork, *PodNetwork, *libcni.RuntimeConf) error

func (plugin *cniNetworkPlugin) forEachNetwork(ctx context.Context, podNetwork *PodNetwork, fromCache bool, actionFn forEachNetworkFn) error {
	plugin.RLock()
	defer plugin.RUnlock()

	if err := plugin.fillPodNetworks(podNetwork); err != nil {
		logrus.Errorf("Error filling interface names: %v", err)

		return err
	}

	if !fromCache {
		// See if we need to re-sync the configuration, which can happen
		// in some racy podman tests. See PR #85.
		missingNetworks := false

		for _, net := range podNetwork.Networks {
			if _, ok := plugin.networks[net.Name]; !ok {
				missingNetworks = true

				break
			}
		}

		if missingNetworks {
			// Need to drop the read lock, as syncNetworkConfig needs write lock.
			// This is safe because we always acquire the pod lock first, *then* the
			// plugin lock, so we're not at risk of deadlock.
			plugin.RUnlock()
			_ = plugin.syncNetworkConfig(ctx) // ignore error; this is best-effort
			plugin.RLock()
		}
	}

	for _, network := range podNetwork.Networks {
		runtimeConfig := podNetwork.RuntimeConfig[network.Name]

		rt, err := buildCNIRuntimeConf(podNetwork, network.Ifname, &runtimeConfig)
		if err != nil {
			logrus.Errorf("Error building CNI runtime config: %v", err)

			return err
		}

		var cniNet *cniNetwork

		if fromCache {
			var newRt *libcni.RuntimeConf

			cniNet, newRt, err = plugin.loadNetworkFromCache(network.Name, rt)
			if err != nil {
				logrus.Errorf("Error loading cached network config: %v", err)
				logrus.Warningf("Falling back to loading from existing plugins on disk")
			} else {
				// Use the updated RuntimeConf
				rt = newRt
			}
		}

		if cniNet == nil {
			cniNet = plugin.networks[network.Name]
			if cniNet == nil {
				return fmt.Errorf("failed to find requested network name %s", network.Name)
			}
		}

		if err := actionFn(cniNet, podNetwork, rt); err != nil {
			return err
		}
	}

	return nil
}

//nolint:gocritic // would be an API change
func (plugin *cniNetworkPlugin) SetUpPod(podNetwork PodNetwork) ([]NetResult, error) {
	return plugin.SetUpPodWithContext(context.Background(), podNetwork)
}

//nolint:gocritic // would be an API change
func (plugin *cniNetworkPlugin) SetUpPodWithContext(ctx context.Context, podNetwork PodNetwork) ([]NetResult, error) {
	if err := plugin.networksAvailable(&podNetwork); err != nil {
		return nil, err
	}

	plugin.gcLock.RLock()
	defer plugin.gcLock.RUnlock()

	plugin.podLock(&podNetwork)
	defer plugin.podUnlock(&podNetwork)

	// Set up loopback interface
	if err := bringUpLoopback(podNetwork.NetNS); err != nil {
		logrus.Error(err)

		return nil, err
	}

	results := make([]NetResult, 0)

	if err := plugin.forEachNetwork(ctx, &podNetwork, false, func(network *cniNetwork, podNetwork *PodNetwork, rt *libcni.RuntimeConf) error {
		fullPodName := buildFullPodName(podNetwork)
		logrus.Infof("Adding pod %s to CNI network %q (type=%v)", fullPodName, network.name, network.config.Plugins[0].Network.Type)
		result, err := network.addToNetwork(ctx, rt, plugin.cniConfig)
		if err != nil {
			return fmt.Errorf("error adding pod %s to CNI network %q: %w", fullPodName, network.name, err)
		}
		results = append(results, NetResult{
			Result: result,
			NetAttachment: NetAttachment{
				Name:   network.name,
				Ifname: rt.IfName,
			},
		})

		return nil
	}); err != nil {
		return nil, err
	}

	return results, nil
}

func (plugin *cniNetworkPlugin) getCachedNetworkInfo(containerID string) ([]NetAttachment, error) {
	cacheDir := libcni.CacheDir
	if plugin.cacheDir != "" {
		cacheDir = plugin.cacheDir
	}

	dirPath := filepath.Join(cacheDir, "results")

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	fileNames := make([]string, 0, len(entries))
	for _, e := range entries {
		fileNames = append(fileNames, e.Name())
	}

	sort.Strings(fileNames)

	attachments := []NetAttachment{}

	for _, fname := range fileNames {
		part := fmt.Sprintf("-%s-", containerID)
		pos := strings.Index(fname, part)

		if pos <= 0 || pos+len(part) >= len(fname) {
			continue
		}

		cacheFile := filepath.Join(dirPath, fname)

		bytes, err := os.ReadFile(cacheFile)
		if err != nil {
			logrus.Errorf("Failed to read CNI cache file %s: %v", cacheFile, err)

			continue
		}

		cachedInfo := struct {
			Kind        string `json:"kind"`
			IfName      string `json:"ifName"`
			ContainerID string `json:"containerID"`
			NetName     string `json:"networkName"`
		}{}

		if err := json.Unmarshal(bytes, &cachedInfo); err != nil {
			logrus.Errorf("Failed to unmarshal CNI cache file %s: %v", cacheFile, err)

			continue
		}

		if cachedInfo.Kind != libcni.CNICacheV1 {
			logrus.Warningf("Unknown CNI cache file %s kind %q", cacheFile, cachedInfo.Kind)

			continue
		}

		if cachedInfo.ContainerID != containerID {
			continue
		}
		// Ignore the loopback interface; it's handled separately
		if cachedInfo.IfName == loIfname && cachedInfo.NetName == "cni-loopback" {
			continue
		}

		if cachedInfo.IfName == "" || cachedInfo.NetName == "" {
			logrus.Warningf("Missing CNI cache file %s ifname %q or netname %q", cacheFile, cachedInfo.IfName, cachedInfo.NetName)

			continue
		}

		attachments = append(attachments, NetAttachment{
			Name:   cachedInfo.NetName,
			Ifname: cachedInfo.IfName,
		})
	}

	return attachments, nil
}

// TearDownPod tears down pod networks. Prefers cached pod attachment information
// but falls back to given network attachment information.
//
//nolint:gocritic // would be an API change
func (plugin *cniNetworkPlugin) TearDownPod(podNetwork PodNetwork) error {
	return plugin.TearDownPodWithContext(context.Background(), podNetwork)
}

//nolint:gocritic // would be an API change
func (plugin *cniNetworkPlugin) TearDownPodWithContext(ctx context.Context, podNetwork PodNetwork) error {
	if len(podNetwork.Networks) == 0 {
		attachments, err := plugin.getCachedNetworkInfo(podNetwork.ID)
		if err == nil && len(attachments) > 0 {
			podNetwork.Networks = attachments
		}
	}

	if err := plugin.networksAvailable(&podNetwork); err != nil {
		return err
	}

	plugin.gcLock.RLock()
	defer plugin.gcLock.RUnlock()

	plugin.podLock(&podNetwork)
	defer plugin.podUnlock(&podNetwork)

	return plugin.forEachNetwork(ctx, &podNetwork, true, func(network *cniNetwork, podNetwork *PodNetwork, rt *libcni.RuntimeConf) error {
		fullPodName := buildFullPodName(podNetwork)

		networkType := "unknown"
		if network.config != nil && len(network.config.Plugins) > 0 && network.config.Plugins[0].Network != nil {
			networkType = network.config.Plugins[0].Network.Type
		}

		logrus.Infof("Deleting pod %s from CNI network %q (type=%v)", fullPodName, network.name, networkType)

		if err := network.deleteFromNetwork(ctx, rt, plugin.cniConfig); err != nil {
			return fmt.Errorf("error removing pod %s from CNI network %q: %w", fullPodName, network.name, err)
		}

		return nil
	})
}

// GetPodNetworkStatus returns IP addressing and interface details for all
// networks attached to the pod.
//
//nolint:gocritic // would be an API change
func (plugin *cniNetworkPlugin) GetPodNetworkStatus(podNetwork PodNetwork) ([]NetResult, error) {
	return plugin.GetPodNetworkStatusWithContext(context.Background(), podNetwork)
}

// GetPodNetworkStatusWithContext returns IP addressing and interface details for all
// networks attached to the pod.
//
//nolint:gocritic // would be an API change
func (plugin *cniNetworkPlugin) GetPodNetworkStatusWithContext(ctx context.Context, podNetwork PodNetwork) ([]NetResult, error) {
	plugin.podLock(&podNetwork)
	defer plugin.podUnlock(&podNetwork)

	if err := checkLoopback(podNetwork.NetNS); err != nil {
		logrus.Error(err)

		return nil, err
	}

	results := make([]NetResult, 0)

	if err := plugin.forEachNetwork(ctx, &podNetwork, true, func(network *cniNetwork, podNetwork *PodNetwork, rt *libcni.RuntimeConf) error {
		fullPodName := buildFullPodName(podNetwork)
		logrus.Infof("Checking pod %s for CNI network %s (type=%v)", fullPodName, network.name, network.config.Plugins[0].Network.Type)
		result, err := network.checkNetwork(ctx, rt, plugin.cniConfig, plugin.nsManager, podNetwork.NetNS)
		if err != nil {
			return fmt.Errorf("error checking pod %s for CNI network %q: %w", fullPodName, network.name, err)
		}
		if result != nil {
			results = append(results, NetResult{
				Result: result,
				NetAttachment: NetAttachment{
					Name:   network.name,
					Ifname: rt.IfName,
				},
			})
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return results, nil
}

// GC cleans up any stale attachments.
// It preserves all attachments and resources belonging to pods in `validPods`. A CNI
// DEL command will be issued for all known cached attachments, then a CNI GC (for CNI
// v1.1 and higher) for any straggling resources.
func (plugin *cniNetworkPlugin) GC(ctx context.Context, validPods []*PodNetwork) error {
	// Must always acquire gcLock before plugin lock.
	plugin.gcLock.Lock()
	defer plugin.gcLock.Unlock()

	// Lock plugin, so we can read config fields.
	plugin.RLock()
	defer plugin.RUnlock()

	// for every network, determine the set of valid attachments -- (ID, ifname) pairs
	validAttachments := map[string][]cnitypes.GCAttachment{}

	for _, pod := range validPods {
		_ = plugin.fillPodNetworks(pod) // cannot have error here; or else pod could not have been ADDed

		for _, network := range pod.Networks {
			validAttachments[network.Name] = append(validAttachments[network.Name], cnitypes.GCAttachment{
				ContainerID: pod.ID,
				IfName:      network.Ifname,
			})
		}
	}

	// For every known network, issue a GC
	var result error

	for netname, network := range plugin.networks {
		args := &libcni.GCArgs{
			ValidAttachments: validAttachments[netname],
		}

		err := network.gcNetwork(ctx, plugin.cniConfig, args)
		if err != nil {
			logrus.Warnf("Error while GCing network %s: %v", netname, err)
			result = errors.Join(result, err)
		}
	}

	return result
}

func (network *cniNetwork) addToNetwork(ctx context.Context, rt *libcni.RuntimeConf, cni *libcni.CNIConfig) (cnitypes.Result, error) {
	return cni.AddNetworkList(ctx, network.config, rt)
}

func (network *cniNetwork) checkNetwork(ctx context.Context, rt *libcni.RuntimeConf, cni *libcni.CNIConfig, nsManager *nsManager, netns string) (cnitypes.Result, error) {
	gtet, err := cniversion.GreaterThanOrEqualTo(network.config.CNIVersion, "0.4.0")
	if err != nil {
		return nil, err
	}

	var result cnitypes.Result

	// When CNIVersion supports Check, use it.  Otherwise fall back on what was done initially.
	if gtet {
		err = cni.CheckNetworkList(ctx, network.config, rt)
		logrus.Infof("Checking CNI network %s (config version=%v)", network.name, network.config.CNIVersion)

		if err != nil {
			logrus.Errorf("Error checking network: %v", err)

			return nil, err
		}
	}

	result, err = cni.GetNetworkListCachedResult(network.config, rt)
	if err != nil {
		logrus.Errorf("Error getting network list cached result: %v", err)

		return nil, err
	} else if result != nil {
		return result, nil
	}

	// result doesn't exist, create one
	logrus.Infof("Checking CNI network %s (config version=%v) nsManager=%v", network.name, network.config.CNIVersion, nsManager)

	var cniInterface *cniv1.Interface

	ips := []*cniv1.IPConfig{}
	errs := []error{}

	for _, version := range []string{"4", "6"} {
		ip, mac, err := getContainerDetails(nsManager, netns, rt.IfName, "-"+version)
		if err == nil {
			if cniInterface == nil {
				cniInterface = &cniv1.Interface{
					Name:    rt.IfName,
					Mac:     mac.String(),
					Sandbox: netns,
				}
			}

			ips = append(ips, &cniv1.IPConfig{
				Interface: cniv1.Int(0),
				Address:   *ip,
			})
		} else {
			errs = append(errs, err)
		}
	}

	if cniInterface == nil || len(ips) == 0 {
		return nil, fmt.Errorf("neither IPv4 nor IPv6 found when retrieving network status: %v", errs)
	}

	result = &cniv1.Result{
		CNIVersion: cniv1.ImplementedSpecVersion,
		Interfaces: []*cniv1.Interface{cniInterface},
		IPs:        ips,
	}

	// Result must be the same CNIVersion as the CNI config
	converted, err := result.GetAsVersion(network.config.CNIVersion)
	if err != nil {
		return nil, err
	}

	return converted, nil
}

func (network *cniNetwork) deleteFromNetwork(ctx context.Context, rt *libcni.RuntimeConf, cni *libcni.CNIConfig) error {
	return cni.DelNetworkList(ctx, network.config, rt)
}

func (network *cniNetwork) getNetworkStatus(ctx context.Context, cni *libcni.CNIConfig) error {
	return cni.GetStatusNetworkList(ctx, network.config)
}

func (network *cniNetwork) gcNetwork(ctx context.Context, cni *libcni.CNIConfig, gcArgs *libcni.GCArgs) error {
	return cni.GCNetworkList(ctx, network.config, gcArgs)
}

func buildCNIRuntimeConf(podNetwork *PodNetwork, ifName string, runtimeConfig *RuntimeConfig) (*libcni.RuntimeConf, error) {
	if runtimeConfig == nil {
		runtimeConfig = &RuntimeConfig{}
	}

	logrus.Infof("Got pod network %+v", podNetwork)

	rt := &libcni.RuntimeConf{
		ContainerID: podNetwork.ID,
		NetNS:       podNetwork.NetNS,
		IfName:      ifName,
		Args: [][2]string{
			{"IgnoreUnknown", "1"},
			{"K8S_POD_NAMESPACE", podNetwork.Namespace},
			{"K8S_POD_NAME", podNetwork.Name},
			{"K8S_POD_INFRA_CONTAINER_ID", podNetwork.ID},
			{"K8S_POD_UID", podNetwork.UID},
		},
		CapabilityArgs: map[string]interface{}{},
	}

	// Propagate existing CNI_ARGS to non-k8s consumers
	for _, kvpairs := range strings.Split(os.Getenv("CNI_ARGS"), ";") {
		if keyval := strings.SplitN(kvpairs, "=", 2); len(keyval) == 2 {
			rt.Args = append(rt.Args, [2]string{keyval[0], keyval[1]})
		}
	}

	// Add requested static IP to CNI_ARGS
	ip := runtimeConfig.IP
	if ip != "" {
		if tstIP := net.ParseIP(ip); tstIP == nil {
			return nil, fmt.Errorf("unable to parse IP address %q", ip)
		}

		rt.Args = append(rt.Args, [2]string{"IP", ip})
	}

	// Add the requested static MAC to CNI_ARGS
	mac := runtimeConfig.MAC
	if mac != "" {
		_, err := net.ParseMAC(mac)
		if err != nil {
			return nil, fmt.Errorf("unable to parse MAC address %q: %w", mac, err)
		}

		rt.Args = append(rt.Args, [2]string{"MAC", mac})
	}

	// Set PortMappings in Capabilities
	if len(runtimeConfig.PortMappings) != 0 {
		rt.CapabilityArgs["portMappings"] = runtimeConfig.PortMappings
	}

	// Set Bandwidth in Capabilities
	if runtimeConfig.Bandwidth != nil {
		rt.CapabilityArgs["bandwidth"] = map[string]uint64{
			"ingressRate":  runtimeConfig.Bandwidth.IngressRate,
			"ingressBurst": runtimeConfig.Bandwidth.IngressBurst,
			"egressRate":   runtimeConfig.Bandwidth.EgressRate,
			"egressBurst":  runtimeConfig.Bandwidth.EgressBurst,
		}
	}

	// Set IpRanges in Capabilities
	if len(runtimeConfig.IpRanges) > 0 {
		rt.CapabilityArgs["ipRanges"] = runtimeConfig.IpRanges
	}

	// Set Aliases in Capabilities
	if len(podNetwork.Aliases) > 0 {
		rt.CapabilityArgs["aliases"] = podNetwork.Aliases
	}

	// set cgroupPath in Capabilities
	if runtimeConfig.CgroupPath != "" {
		rt.CapabilityArgs["cgroupPath"] = runtimeConfig.CgroupPath
	}

	// Set PodAnnotations in Capabilities
	if runtimeConfig.PodAnnotations != nil {
		rt.CapabilityArgs["io.kubernetes.cri.pod-annotations"] = runtimeConfig.PodAnnotations
	}

	return rt, nil
}

// Status returns error if the default network is not configured, or if
// the default network reports a failing STATUS code.
func (plugin *cniNetworkPlugin) Status() error {
	return plugin.StatusWithContext(context.Background())
}

func (plugin *cniNetworkPlugin) StatusWithContext(ctx context.Context) error {
	defaultNet := plugin.getDefaultNetwork()
	if defaultNet == nil {
		return fmt.Errorf(errMissingDefaultNetwork, plugin.confDir)
	}

	return defaultNet.getNetworkStatus(ctx, plugin.cniConfig)
}
