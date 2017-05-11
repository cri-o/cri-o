/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package app does all of the work necessary to configure and run a
// Kubernetes app process.
package app

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoclientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	clientv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubernetes/cmd/kube-proxy/app/options"
	"k8s.io/kubernetes/pkg/api"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/proxy"
	proxyconfig "k8s.io/kubernetes/pkg/proxy/config"
	"k8s.io/kubernetes/pkg/proxy/iptables"
	"k8s.io/kubernetes/pkg/proxy/userspace"
	"k8s.io/kubernetes/pkg/proxy/winuserspace"
	"k8s.io/kubernetes/pkg/util/configz"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	"k8s.io/kubernetes/pkg/util/exec"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	utilnetsh "k8s.io/kubernetes/pkg/util/netsh"
	nodeutil "k8s.io/kubernetes/pkg/util/node"
	"k8s.io/kubernetes/pkg/util/oom"
	"k8s.io/kubernetes/pkg/util/resourcecontainer"
	utilsysctl "k8s.io/kubernetes/pkg/util/sysctl"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type ProxyServer struct {
	Client       clientset.Interface
	EventClient  v1core.EventsGetter
	Config       *options.ProxyServerConfig
	IptInterface utiliptables.Interface
	Proxier      proxy.ProxyProvider
	Broadcaster  record.EventBroadcaster
	Recorder     record.EventRecorder
	Conntracker  Conntracker // if nil, ignored
	ProxyMode    string
}

const (
	proxyModeUserspace = "userspace"
	proxyModeIPTables  = "iptables"
)

func checkKnownProxyMode(proxyMode string) bool {
	switch proxyMode {
	case "", proxyModeUserspace, proxyModeIPTables:
		return true
	}
	return false
}

func NewProxyServer(
	client clientset.Interface,
	eventClient v1core.EventsGetter,
	config *options.ProxyServerConfig,
	iptInterface utiliptables.Interface,
	proxier proxy.ProxyProvider,
	broadcaster record.EventBroadcaster,
	recorder record.EventRecorder,
	conntracker Conntracker,
	proxyMode string,
) (*ProxyServer, error) {
	return &ProxyServer{
		Client:       client,
		EventClient:  eventClient,
		Config:       config,
		IptInterface: iptInterface,
		Proxier:      proxier,
		Broadcaster:  broadcaster,
		Recorder:     recorder,
		Conntracker:  conntracker,
		ProxyMode:    proxyMode,
	}, nil
}

// NewProxyCommand creates a *cobra.Command object with default parameters
func NewProxyCommand() *cobra.Command {
	s := options.NewProxyConfig()
	s.AddFlags(pflag.CommandLine)
	cmd := &cobra.Command{
		Use: "kube-proxy",
		Long: `The Kubernetes network proxy runs on each node. This
reflects services as defined in the Kubernetes API on each node and can do simple
TCP,UDP stream forwarding or round robin TCP,UDP forwarding across a set of backends.
Service cluster ips and ports are currently found through Docker-links-compatible
environment variables specifying ports opened by the service proxy. There is an optional
addon that provides cluster DNS for these cluster IPs. The user must create a service
with the apiserver API to configure the proxy.`,
		Run: func(cmd *cobra.Command, args []string) {
		},
	}

	return cmd
}

// NewProxyServerDefault creates a new ProxyServer object with default parameters.
func NewProxyServerDefault(config *options.ProxyServerConfig) (*ProxyServer, error) {
	if c, err := configz.New("componentconfig"); err == nil {
		c.Set(config.KubeProxyConfiguration)
	} else {
		glog.Errorf("unable to register configz: %s", err)
	}
	protocol := utiliptables.ProtocolIpv4
	if net.ParseIP(config.BindAddress).To4() == nil {
		protocol = utiliptables.ProtocolIpv6
	}

	var netshInterface utilnetsh.Interface
	var iptInterface utiliptables.Interface
	var dbus utildbus.Interface

	// Create a iptables utils.
	execer := exec.New()

	if runtime.GOOS == "windows" {
		netshInterface = utilnetsh.New(execer)
	} else {
		dbus = utildbus.New()
		iptInterface = utiliptables.New(execer, dbus, protocol)
	}

	// We omit creation of pretty much everything if we run in cleanup mode
	if config.CleanupAndExit {
		return &ProxyServer{
			Config:       config,
			IptInterface: iptInterface,
		}, nil
	}

	// TODO(vmarmol): Use container config for this.
	var oomAdjuster *oom.OOMAdjuster
	if config.OOMScoreAdj != nil {
		oomAdjuster = oom.NewOOMAdjuster()
		if err := oomAdjuster.ApplyOOMScoreAdj(0, int(*config.OOMScoreAdj)); err != nil {
			glog.V(2).Info(err)
		}
	}

	if config.ResourceContainer != "" {
		// Run in its own container.
		if err := resourcecontainer.RunInResourceContainer(config.ResourceContainer); err != nil {
			glog.Warningf("Failed to start in resource-only container %q: %v", config.ResourceContainer, err)
		} else {
			glog.V(2).Infof("Running in resource-only container %q", config.ResourceContainer)
		}
	}

	// Create a Kube Client
	// define api config source
	if config.Kubeconfig == "" && config.Master == "" {
		glog.Warningf("Neither --kubeconfig nor --master was specified.  Using default API client.  This might not work.")
	}
	// This creates a client, first loading any specified kubeconfig
	// file, and then overriding the Master flag, if non-empty.
	kubeconfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: config.Kubeconfig},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: config.Master}}).ClientConfig()
	if err != nil {
		return nil, err
	}

	kubeconfig.ContentType = config.ContentType
	// Override kubeconfig qps/burst settings from flags
	kubeconfig.QPS = config.KubeAPIQPS
	kubeconfig.Burst = int(config.KubeAPIBurst)

	client, err := clientset.NewForConfig(kubeconfig)
	if err != nil {
		glog.Fatalf("Invalid API configuration: %v", err)
	}

	eventClient, err := clientgoclientset.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	// Create event recorder
	hostname := nodeutil.GetHostname(config.HostnameOverride)
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(api.Scheme, clientv1.EventSource{Component: "kube-proxy", Host: hostname})

	var proxier proxy.ProxyProvider
	var endpointsHandler proxyconfig.EndpointsConfigHandler

	proxyMode := getProxyMode(string(config.Mode), client.Core().Nodes(), hostname, iptInterface, iptables.LinuxKernelCompatTester{})
	if proxyMode == proxyModeIPTables {
		glog.V(0).Info("Using iptables Proxier.")
		if config.IPTablesMasqueradeBit == nil {
			// IPTablesMasqueradeBit must be specified or defaulted.
			return nil, fmt.Errorf("Unable to read IPTablesMasqueradeBit from config")
		}
		proxierIPTables, err := iptables.NewProxier(
			iptInterface,
			utilsysctl.New(),
			execer,
			config.IPTablesSyncPeriod.Duration,
			config.IPTablesMinSyncPeriod.Duration,
			config.MasqueradeAll,
			int(*config.IPTablesMasqueradeBit),
			config.ClusterCIDR,
			hostname,
			getNodeIP(client, hostname),
			recorder,
		)
		if err != nil {
			glog.Fatalf("Unable to create proxier: %v", err)
		}
		proxier = proxierIPTables
		endpointsHandler = proxierIPTables
		// No turning back. Remove artifacts that might still exist from the userspace Proxier.
		glog.V(0).Info("Tearing down userspace rules.")
		userspace.CleanupLeftovers(iptInterface)
	} else {
		glog.V(0).Info("Using userspace Proxier.")

		var proxierUserspace proxy.ProxyProvider

		if runtime.GOOS == "windows" {
			// This is a proxy.LoadBalancer which NewProxier needs but has methods we don't need for
			// our config.EndpointsConfigHandler.
			loadBalancer := winuserspace.NewLoadBalancerRR()
			// set EndpointsConfigHandler to our loadBalancer
			endpointsHandler = loadBalancer
			proxierUserspace, err = winuserspace.NewProxier(
				loadBalancer,
				net.ParseIP(config.BindAddress),
				netshInterface,
				*utilnet.ParsePortRangeOrDie(config.PortRange),
				// TODO @pires replace below with default values, if applicable
				config.IPTablesSyncPeriod.Duration,
				config.UDPIdleTimeout.Duration,
			)
		} else {
			// This is a proxy.LoadBalancer which NewProxier needs but has methods we don't need for
			// our config.EndpointsConfigHandler.
			loadBalancer := userspace.NewLoadBalancerRR()
			// set EndpointsConfigHandler to our loadBalancer
			endpointsHandler = loadBalancer
			proxierUserspace, err = userspace.NewProxier(
				loadBalancer,
				net.ParseIP(config.BindAddress),
				iptInterface,
				execer,
				*utilnet.ParsePortRangeOrDie(config.PortRange),
				config.IPTablesSyncPeriod.Duration,
				config.IPTablesMinSyncPeriod.Duration,
				config.UDPIdleTimeout.Duration,
			)
		}
		if err != nil {
			glog.Fatalf("Unable to create proxier: %v", err)
		}
		proxier = proxierUserspace
		// Remove artifacts from the pure-iptables Proxier, if not on Windows.
		if runtime.GOOS != "windows" {
			glog.V(0).Info("Tearing down pure-iptables proxy rules.")
			iptables.CleanupLeftovers(iptInterface)
		}
	}

	// Add iptables reload function, if not on Windows.
	if runtime.GOOS != "windows" {
		iptInterface.AddReloadFunc(proxier.Sync)
	}

	// Create configs (i.e. Watches for Services and Endpoints)
	// Note: RegisterHandler() calls need to happen before creation of Sources because sources
	// only notify on changes, and the initial update (on process start) may be lost if no handlers
	// are registered yet.
	serviceConfig := proxyconfig.NewServiceConfig()
	serviceConfig.RegisterHandler(proxier)

	endpointsConfig := proxyconfig.NewEndpointsConfig()
	endpointsConfig.RegisterHandler(endpointsHandler)

	proxyconfig.NewSourceAPI(
		client.Core().RESTClient(),
		config.ConfigSyncPeriod,
		serviceConfig.Channel("api"),
		endpointsConfig.Channel("api"),
	)

	config.NodeRef = &clientv1.ObjectReference{
		Kind:      "Node",
		Name:      hostname,
		UID:       types.UID(hostname),
		Namespace: "",
	}

	conntracker := realConntracker{}

	return NewProxyServer(client, eventClient, config, iptInterface, proxier, eventBroadcaster, recorder, conntracker, proxyMode)
}

// Run runs the specified ProxyServer.  This should never exit (unless CleanupAndExit is set).
func (s *ProxyServer) Run() error {
	// remove iptables rules and exit
	if s.Config.CleanupAndExit {
		encounteredError := userspace.CleanupLeftovers(s.IptInterface)
		encounteredError = iptables.CleanupLeftovers(s.IptInterface) || encounteredError
		if encounteredError {
			return errors.New("Encountered an error while tearing down rules.")
		}
		return nil
	}

	s.Broadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: s.EventClient.Events("")})

	// Start up a webserver if requested
	if s.Config.HealthzPort > 0 {
		http.HandleFunc("/proxyMode", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "%s", s.ProxyMode)
		})
		http.Handle("/metrics", prometheus.Handler())
		configz.InstallHandler(http.DefaultServeMux)
		go wait.Until(func() {
			err := http.ListenAndServe(s.Config.HealthzBindAddress+":"+strconv.Itoa(int(s.Config.HealthzPort)), nil)
			if err != nil {
				glog.Errorf("Starting health server failed: %v", err)
			}
		}, 5*time.Second, wait.NeverStop)
	}

	// Tune conntrack, if requested
	if s.Conntracker != nil && runtime.GOOS != "windows" {
		max, err := getConntrackMax(s.Config)
		if err != nil {
			return err
		}
		if max > 0 {
			err := s.Conntracker.SetMax(max)
			if err != nil {
				if err != readOnlySysFSError {
					return err
				}
				// readOnlySysFSError is caused by a known docker issue (https://github.com/docker/docker/issues/24000),
				// the only remediation we know is to restart the docker daemon.
				// Here we'll send an node event with specific reason and message, the
				// administrator should decide whether and how to handle this issue,
				// whether to drain the node and restart docker.
				// TODO(random-liu): Remove this when the docker bug is fixed.
				const message = "DOCKER RESTART NEEDED (docker issue #24000): /sys is read-only: " +
					"cannot modify conntrack limits, problems may arise later."
				s.Recorder.Eventf(s.Config.NodeRef, api.EventTypeWarning, err.Error(), message)
			}
		}

		if s.Config.ConntrackTCPEstablishedTimeout.Duration > 0 {
			timeout := int(s.Config.ConntrackTCPEstablishedTimeout.Duration / time.Second)
			if err := s.Conntracker.SetTCPEstablishedTimeout(timeout); err != nil {
				return err
			}
		}

		if s.Config.ConntrackTCPCloseWaitTimeout.Duration > 0 {
			timeout := int(s.Config.ConntrackTCPCloseWaitTimeout.Duration / time.Second)
			if err := s.Conntracker.SetTCPCloseWaitTimeout(timeout); err != nil {
				return err
			}
		}
	}

	// Birth Cry after the birth is successful
	s.birthCry()

	// Just loop forever for now...
	s.Proxier.SyncLoop()
	return nil
}

func getConntrackMax(config *options.ProxyServerConfig) (int, error) {
	if config.ConntrackMax > 0 {
		if config.ConntrackMaxPerCore > 0 {
			return -1, fmt.Errorf("invalid config: ConntrackMax and ConntrackMaxPerCore are mutually exclusive")
		}
		glog.V(3).Infof("getConntrackMax: using absolute conntrack-max (deprecated)")
		return int(config.ConntrackMax), nil
	}
	if config.ConntrackMaxPerCore > 0 {
		floor := int(config.ConntrackMin)
		scaled := int(config.ConntrackMaxPerCore) * runtime.NumCPU()
		if scaled > floor {
			glog.V(3).Infof("getConntrackMax: using scaled conntrack-max-per-core")
			return scaled, nil
		}
		glog.V(3).Infof("getConntrackMax: using conntrack-min")
		return floor, nil
	}
	return 0, nil
}

type nodeGetter interface {
	Get(hostname string, options metav1.GetOptions) (*api.Node, error)
}

func getProxyMode(proxyMode string, client nodeGetter, hostname string, iptver iptables.IPTablesVersioner, kcompat iptables.KernelCompatTester) string {
	if proxyMode == proxyModeUserspace {
		return proxyModeUserspace
	} else if proxyMode == proxyModeIPTables {
		return tryIPTablesProxy(iptver, kcompat)
	} else if proxyMode != "" {
		glog.Warningf("Flag proxy-mode=%q unknown, assuming iptables proxy", proxyMode)
		return tryIPTablesProxy(iptver, kcompat)
	}
	return tryIPTablesProxy(iptver, kcompat)
}

func tryIPTablesProxy(iptver iptables.IPTablesVersioner, kcompat iptables.KernelCompatTester) string {
	// guaranteed false on error, error only necessary for debugging
	useIPTablesProxy, err := iptables.CanUseIPTablesProxier(iptver, kcompat)
	if err != nil {
		glog.Errorf("Can't determine whether to use iptables proxy, using userspace proxier: %v", err)
		return proxyModeUserspace
	}
	if useIPTablesProxy {
		return proxyModeIPTables
	}
	// Fallback.
	glog.V(1).Infof("Can't use iptables proxy, using userspace proxier")
	return proxyModeUserspace
}

func (s *ProxyServer) birthCry() {
	s.Recorder.Eventf(s.Config.NodeRef, api.EventTypeNormal, "Starting", "Starting kube-proxy.")
}

func getNodeIP(client clientset.Interface, hostname string) net.IP {
	var nodeIP net.IP
	node, err := client.Core().Nodes().Get(hostname, metav1.GetOptions{})
	if err != nil {
		glog.Warningf("Failed to retrieve node info: %v", err)
		return nil
	}
	nodeIP, err = nodeutil.InternalGetNodeHostIP(node)
	if err != nil {
		glog.Warningf("Failed to retrieve node IP: %v", err)
		return nil
	}
	return nodeIP
}
