package ocicni

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/fsnotify/fsnotify"
)

type cniNetworkPlugin struct {
	loNetwork *cniNetwork

	sync.RWMutex
	defaultNetwork *cniNetwork

	nsenterPath        string
	pluginDir          string
	cniDirs            []string
	vendorCNIDirPrefix string

	cniReadyListeners []chan error

	monitorNetDirChan chan error
	monitorNetDirDone bool
}

type cniNetwork struct {
	name          string
	NetworkConfig *libcni.NetworkConfig
	CNIConfig     libcni.CNI
}

var (
	errMissingDefaultNetwork       = errors.New("Missing CNI default network")
	errDefaultNetworkAlreadyExists = errors.New("CNI default network already exists")
	errMonitoringTimeout           = errors.New("CNI monitoring timeout")
)

func (plugin *cniNetworkPlugin) monitorNetDir() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Errorf("could not create new watcher %v", err)
		plugin.notifyCniReadyListeners(err)
		return
	}
	defer watcher.Close()

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				logrus.Debugf("CNI monitoring event %v", event)
				if event.Op&fsnotify.Create != fsnotify.Create {
					continue
				}

				logrus.Debugf("CNI asynchronous setting succeeded")
				plugin.terminateMonitorNetDir(nil)
				return

			case err1 := <-watcher.Errors:
				logrus.Errorf("CNI monitoring error %v", err1)
				plugin.terminateMonitorNetDir(err1)
				return
			}
		}
	}()

	if err = watcher.Add(plugin.pluginDir); err != nil {
		logrus.Error(err)
		plugin.notifyCniReadyListeners(err)
		return
	}

	err = <-plugin.monitorNetDirChan
	plugin.notifyCniReadyListeners(err)
}

// InitCNI takes the plugin directory and cni directories where the cni files should be searched for
// Returns a valid plugin object and any error
func InitCNI(pluginDir string, cniDirs ...string) (CNIPlugin, error) {
	plugin := probeNetworkPluginsWithVendorCNIDirPrefix(pluginDir, cniDirs, "")
	var err error
	plugin.nsenterPath, err = exec.LookPath("nsenter")
	if err != nil {
		return nil, err
	}

	// check if a default network exists, otherwise dump the CNI search and return a noop plugin
	_, err = getDefaultCNINetwork(plugin.pluginDir, plugin.cniDirs, plugin.vendorCNIDirPrefix)
	if err != nil {
		if err == errMissingDefaultNetwork {
			go plugin.monitorNetDir()
			return plugin, nil
		}

		logrus.Warningf("Error in finding usable CNI plugin - %v", err)
		// create a noop plugin instead
		return &cniNoOp{}, nil
	}

	return plugin, nil
}

func probeNetworkPluginsWithVendorCNIDirPrefix(pluginDir string, cniDirs []string, vendorCNIDirPrefix string) *cniNetworkPlugin {
	plugin := &cniNetworkPlugin{
		defaultNetwork:     nil,
		loNetwork:          getLoNetwork(cniDirs, vendorCNIDirPrefix),
		pluginDir:          pluginDir,
		cniDirs:            cniDirs,
		vendorCNIDirPrefix: vendorCNIDirPrefix,
		monitorNetDirChan:  make(chan error),
		monitorNetDirDone:  false,
	}

	// sync NetworkConfig in best effort during probing.
	plugin.syncNetworkConfig()
	return plugin
}

func getDefaultCNINetwork(pluginDir string, cniDirs []string, vendorCNIDirPrefix string) (*cniNetwork, error) {
	if pluginDir == "" {
		pluginDir = DefaultNetDir
	}
	if len(cniDirs) == 0 {
		cniDirs = []string{DefaultCNIDir}
	}

	files, err := libcni.ConfFiles(pluginDir)
	switch {
	case err != nil:
		return nil, err
	case len(files) == 0:
		return nil, errMissingDefaultNetwork
	}

	sort.Strings(files)
	for _, confFile := range files {
		conf, err := libcni.ConfFromFile(confFile)
		if err != nil {
			logrus.Warningf("Error loading CNI config file %s: %v", confFile, err)
			continue
		}

		// Search for vendor-specific plugins as well as default plugins in the CNI codebase.
		vendorDir := vendorCNIDir(vendorCNIDirPrefix, conf.Network.Type)
		cninet := &libcni.CNIConfig{
			Path: append(cniDirs, vendorDir),
		}

		network := &cniNetwork{name: conf.Network.Name, NetworkConfig: conf, CNIConfig: cninet}
		return network, nil
	}
	return nil, fmt.Errorf("No valid networks found in %s", pluginDir)
}

func vendorCNIDir(prefix, pluginType string) string {
	return fmt.Sprintf(VendorCNIDirTemplate, prefix, pluginType)
}

func getLoNetwork(cniDirs []string, vendorDirPrefix string) *cniNetwork {
	if len(cniDirs) == 0 {
		cniDirs = []string{DefaultCNIDir}
	}

	loConfig, err := libcni.ConfFromBytes([]byte(`{
  "cniVersion": "0.1.0",
  "name": "cni-loopback",
  "type": "loopback"
}`))
	if err != nil {
		// The hardcoded config above should always be valid and unit tests will
		// catch this
		panic(err)
	}
	vendorDir := vendorCNIDir(vendorDirPrefix, loConfig.Network.Type)
	cninet := &libcni.CNIConfig{
		Path: append(cniDirs, vendorDir),
	}
	loNetwork := &cniNetwork{
		name:          "lo",
		NetworkConfig: loConfig,
		CNIConfig:     cninet,
	}

	return loNetwork
}

func (plugin *cniNetworkPlugin) syncNetworkConfig() {
	network, err := getDefaultCNINetwork(plugin.pluginDir, plugin.cniDirs, plugin.vendorCNIDirPrefix)
	if err != nil {
		logrus.Errorf("error updating cni config: %s", err)
		return
	}
	plugin.setDefaultNetwork(network)
}

func (plugin *cniNetworkPlugin) getDefaultNetwork() *cniNetwork {
	plugin.RLock()
	defer plugin.RUnlock()
	return plugin.defaultNetwork
}

func (plugin *cniNetworkPlugin) setDefaultNetwork(n *cniNetwork) {
	plugin.Lock()
	defer plugin.Unlock()
	plugin.defaultNetwork = n
}

func (plugin *cniNetworkPlugin) terminateMonitorNetDir(err error) {
	plugin.Lock()
	defer plugin.Unlock()
	if !plugin.monitorNetDirDone {
		plugin.monitorNetDirChan <- err
		plugin.monitorNetDirDone = true
	}
}

func (plugin *cniNetworkPlugin) addCniReadyListener() (chan error, error) {
	plugin.Lock()
	defer plugin.Unlock()

	if plugin.defaultNetwork != nil {
		return nil, errDefaultNetworkAlreadyExists
	}

	c := make(chan error)
	plugin.cniReadyListeners = append(plugin.cniReadyListeners, c)

	return c, nil
}

func (plugin *cniNetworkPlugin) removeCniReadyListener(c chan error) {
	plugin.Lock()
	defer plugin.Unlock()

	for i, ch := range plugin.cniReadyListeners {
		if c != ch {
			continue
		}

		plugin.cniReadyListeners = append(plugin.cniReadyListeners[:i], plugin.cniReadyListeners[i+1:]...)
		break
	}

	return
}

func (plugin *cniNetworkPlugin) notifyCniReadyListeners(err error) {
	plugin.Lock()
	defer plugin.Unlock()

	for _, c := range plugin.cniReadyListeners {
		c <- err
	}

	plugin.cniReadyListeners = nil
}

func (plugin *cniNetworkPlugin) checkInitialized() error {
	if plugin.getDefaultNetwork() == nil {
		return errMissingDefaultNetwork
	}
	return nil
}

func (plugin *cniNetworkPlugin) Name() string {
	return CNIPluginName
}

func (plugin *cniNetworkPlugin) setUpPod(netnsPath string, namespace string, name string, id string) error {
	plugin.syncNetworkConfig()

	if err := plugin.checkInitialized(); err != nil {
		return err
	}

	_, err := plugin.loNetwork.addToNetwork(name, namespace, id, netnsPath)
	if err != nil {
		logrus.Errorf("Error while adding to cni lo network: %s", err)
		return err
	}

	_, err = plugin.getDefaultNetwork().addToNetwork(name, namespace, id, netnsPath)
	if err != nil {
		logrus.Errorf("Error while adding to cni network: %s", err)
		return err
	}

	return err

}

func (plugin *cniNetworkPlugin) SetUpPod(netnsPath string, namespace string, name string, id string, cb SetUpCallback, data interface{}) error {
	// First let's sync with the latest configuration files
	plugin.syncNetworkConfig()

	// Now we can check if we really have a default network
	if err := plugin.checkInitialized(); err != nil {
		if err == errMissingDefaultNetwork {
			// We are missing a default network.
			// Let's add ourselves to the listeners list and
			// wait 30s for a new configuration file to show up.
			c, err1 := plugin.addCniReadyListener()
			if err1 != nil {
				// The CNI default network showed up
				if err1 == errDefaultNetworkAlreadyExists {
					return plugin.setUpPod(netnsPath, namespace, name, id)
				}

				return err1
			}

			go func() {
				select {
				case err2 := <-c:
					cb(data, err2)

				case <-time.After(time.Second * 30):
					cb(data, errMonitoringTimeout)
				}

				plugin.removeCniReadyListener(c)
			}()

			return nil
		}

		return err
	}

	return plugin.setUpPod(netnsPath, namespace, name, id)
}

func (plugin *cniNetworkPlugin) TearDownPod(netnsPath string, namespace string, name string, id string) error {
	if err := plugin.checkInitialized(); err != nil {
		if err == errMissingDefaultNetwork {
			// We are missing a default network but someone is still
			// trying to tear us down. We will try to kill the monitoring
			// thread if it's still running.
			plugin.terminateMonitorNetDir(nil)
			return nil
		}

		return err
	}

	return plugin.getDefaultNetwork().deleteFromNetwork(name, namespace, id, netnsPath)
}

// TODO: Use the addToNetwork function to obtain the IP of the Pod. That will assume idempotent ADD call to the plugin.
// Also fix the runtime's call to Status function to be done only in the case that the IP is lost, no need to do periodic calls
func (plugin *cniNetworkPlugin) GetContainerNetworkStatus(netnsPath string, namespace string, name string, id string) (string, error) {
	ip, err := getContainerIP(plugin.nsenterPath, netnsPath, DefaultInterfaceName, "-4")
	if err != nil {
		return "", err
	}

	return ip.String(), nil
}

func (network *cniNetwork) addToNetwork(podName string, podNamespace string, podInfraContainerID string, podNetnsPath string) (*cnitypes.Result, error) {
	rt, err := buildCNIRuntimeConf(podName, podNamespace, podInfraContainerID, podNetnsPath)
	if err != nil {
		logrus.Errorf("Error adding network: %v", err)
		return nil, err
	}

	netconf, cninet := network.NetworkConfig, network.CNIConfig
	logrus.Infof("About to run with conf.Network.Type=%v", netconf.Network.Type)
	res, err := cninet.AddNetwork(netconf, rt)
	if err != nil {
		logrus.Errorf("Error adding network: %v", err)
		return nil, err
	}

	return res, nil
}

func (network *cniNetwork) deleteFromNetwork(podName string, podNamespace string, podInfraContainerID string, podNetnsPath string) error {
	rt, err := buildCNIRuntimeConf(podName, podNamespace, podInfraContainerID, podNetnsPath)
	if err != nil {
		logrus.Errorf("Error deleting network: %v", err)
		return err
	}

	netconf, cninet := network.NetworkConfig, network.CNIConfig
	logrus.Infof("About to run with conf.Network.Type=%v", netconf.Network.Type)
	err = cninet.DelNetwork(netconf, rt)
	if err != nil {
		logrus.Errorf("Error deleting network: %v", err)
		return err
	}
	return nil
}

func buildCNIRuntimeConf(podName string, podNs string, podInfraContainerID string, podNetnsPath string) (*libcni.RuntimeConf, error) {
	logrus.Infof("Got netns path %v", podNetnsPath)
	logrus.Infof("Using netns path %v", podNs)

	rt := &libcni.RuntimeConf{
		ContainerID: podInfraContainerID,
		NetNS:       podNetnsPath,
		IfName:      DefaultInterfaceName,
		Args: [][2]string{
			{"IgnoreUnknown", "1"},
			{"K8S_POD_NAMESPACE", podNs},
			{"K8S_POD_NAME", podName},
			{"K8S_POD_INFRA_CONTAINER_ID", podInfraContainerID},
		},
	}

	return rt, nil
}

func (plugin *cniNetworkPlugin) Status() error {
	return plugin.checkInitialized()
}
