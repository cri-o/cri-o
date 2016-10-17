package ocicni

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"sync"
	"time"

	"github.com/containernetworking/cni/libcni"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/golang/glog"
)

type cniNetworkPlugin struct {
	loNetwork *cniNetwork

	sync.RWMutex
	defaultNetwork *cniNetwork

	nsenterPath        string
	pluginDir          string
	cniDirs            []string
	vendorCNIDirPrefix string
}

type cniNetwork struct {
	name          string
	NetworkConfig *libcni.NetworkConfig
	CNIConfig     libcni.CNI
}

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
		glog.Warningf("Error in finding usable CNI plugin - %v", err)
		// create a noop plugin instead
		return &cniNoOp{}, nil
	}

	// sync network config from pluginDir periodically to detect network config updates
	go func() {
		t := time.NewTimer(10 * time.Second)
		for {
			plugin.syncNetworkConfig()
			select {
			case <-t.C:
			}
		}
	}()
	return plugin, nil
}

func probeNetworkPluginsWithVendorCNIDirPrefix(pluginDir string, cniDirs []string, vendorCNIDirPrefix string) *cniNetworkPlugin {
	plugin := &cniNetworkPlugin{
		defaultNetwork:     nil,
		loNetwork:          getLoNetwork(cniDirs, vendorCNIDirPrefix),
		pluginDir:          pluginDir,
		cniDirs:            cniDirs,
		vendorCNIDirPrefix: vendorCNIDirPrefix,
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
		return nil, fmt.Errorf("No networks found in %s", pluginDir)
	}

	sort.Strings(files)
	for _, confFile := range files {
		conf, err := libcni.ConfFromFile(confFile)
		if err != nil {
			glog.Warningf("Error loading CNI config file %s: %v", confFile, err)
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
		glog.Errorf("error updating cni config: %s", err)
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

func (plugin *cniNetworkPlugin) checkInitialized() error {
	if plugin.getDefaultNetwork() == nil {
		return errors.New("cni config uninitialized")
	}
	return nil
}

func (plugin *cniNetworkPlugin) Name() string {
	return CNIPluginName
}

func (plugin *cniNetworkPlugin) SetUpPod(netnsPath string, namespace string, name string, id string) error {
	if err := plugin.checkInitialized(); err != nil {
		return err
	}

	_, err := plugin.loNetwork.addToNetwork(name, namespace, id, netnsPath)
	if err != nil {
		glog.Errorf("Error while adding to cni lo network: %s", err)
		return err
	}

	_, err = plugin.getDefaultNetwork().addToNetwork(name, namespace, id, netnsPath)
	if err != nil {
		glog.Errorf("Error while adding to cni network: %s", err)
		return err
	}

	return err
}

func (plugin *cniNetworkPlugin) TearDownPod(netnsPath string, namespace string, name string, id string) error {
	if err := plugin.checkInitialized(); err != nil {
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
		glog.Errorf("Error adding network: %v", err)
		return nil, err
	}

	netconf, cninet := network.NetworkConfig, network.CNIConfig
	glog.V(4).Infof("About to run with conf.Network.Type=%v", netconf.Network.Type)
	res, err := cninet.AddNetwork(netconf, rt)
	if err != nil {
		glog.Errorf("Error adding network: %v", err)
		return nil, err
	}

	return res, nil
}

func (network *cniNetwork) deleteFromNetwork(podName string, podNamespace string, podInfraContainerID string, podNetnsPath string) error {
	rt, err := buildCNIRuntimeConf(podName, podNamespace, podInfraContainerID, podNetnsPath)
	if err != nil {
		glog.Errorf("Error deleting network: %v", err)
		return err
	}

	netconf, cninet := network.NetworkConfig, network.CNIConfig
	glog.V(4).Infof("About to run with conf.Network.Type=%v", netconf.Network.Type)
	err = cninet.DelNetwork(netconf, rt)
	if err != nil {
		glog.Errorf("Error deleting network: %v", err)
		return err
	}
	return nil
}

func buildCNIRuntimeConf(podName string, podNs string, podInfraContainerID string, podNetnsPath string) (*libcni.RuntimeConf, error) {
	glog.V(4).Infof("Got netns path %v", podNetnsPath)
	glog.V(4).Infof("Using netns path %v", podNs)

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
