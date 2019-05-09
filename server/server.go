package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containers/libpod/pkg/apparmor"
	"github.com/containers/storage/pkg/idtools"
	"github.com/cri-o/cri-o/lib"
	"github.com/cri-o/cri-o/lib/sandbox"
	"github.com/cri-o/cri-o/oci"
	"github.com/cri-o/cri-o/pkg/seccomp"
	"github.com/cri-o/cri-o/pkg/storage"
	"github.com/cri-o/cri-o/server/metrics"
	"github.com/cri-o/cri-o/version"
	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/fsnotify/fsnotify"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	knet "k8s.io/apimachinery/pkg/util/net"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
	"k8s.io/kubernetes/pkg/kubelet/server/streaming"
	iptablesproxy "k8s.io/kubernetes/pkg/proxy/iptables"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	utilexec "k8s.io/utils/exec"
)

const (
	shutdownFile        = "/var/lib/crio/crio.shutdown"
	certRefreshInterval = time.Minute * 5

	apparmorDefaultProfile  = "crio-default-" + version.Version
	apparmorRuntimeDefault  = "runtime/default"
	apparmorLocalHostPrefix = "localhost/"
)

// StreamService implements streaming.Runtime.
type StreamService struct {
	runtimeServer       *Server // needed by Exec() endpoint
	streamServer        streaming.Server
	streamServerCloseCh chan struct{}
	streaming.Runtime
}

// Server implements the RuntimeService and ImageService
type Server struct {
	config          Config
	seccompProfile  seccomp.Seccomp
	stream          StreamService
	netPlugin       ocicni.CNIPlugin
	hostportManager hostport.HostPortManager

	appArmorProfile string
	hostIP          string
	bindAddress     string

	*lib.ContainerServer
	monitorsChan      chan struct{}
	defaultIDMappings *idtools.IDMappings

	updateLock sync.RWMutex

	seccompEnabled  bool
	appArmorEnabled bool
}

type certConfigCache struct {
	config  *tls.Config
	expires time.Time

	tlsCert string
	tlsKey  string
	tlsCA   string
}

// GetConfigForClient gets the tlsConfig for the streaming server.
// This allows the certs to be swapped, without shutting down crio.
func (cc *certConfigCache) GetConfigForClient(hello *tls.ClientHelloInfo) (*tls.Config, error) {
	if cc.config != nil && time.Now().Before(cc.expires) {
		return cc.config, nil
	}
	config := new(tls.Config)
	cert, err := tls.LoadX509KeyPair(cc.tlsCert, cc.tlsKey)
	if err != nil {
		return nil, err
	}
	config.Certificates = []tls.Certificate{cert}
	if len(cc.tlsCA) > 0 {
		caBytes, err := ioutil.ReadFile(cc.tlsCA)
		if err != nil {
			return nil, err
		}
		certPool := x509.NewCertPool()
		certPool.AppendCertsFromPEM(caBytes)
		config.ClientCAs = certPool
		config.ClientAuth = tls.RequireAndVerifyClientCert
	}
	cc.config = config
	cc.expires = time.Now().Add(certRefreshInterval)
	return config, nil
}

// StopStreamServer stops the stream server
func (s *Server) StopStreamServer() error {
	return s.stream.streamServer.Stop()
}

// StreamingServerCloseChan returns the close channel for the streaming server
func (s *Server) StreamingServerCloseChan() chan struct{} {
	return s.stream.streamServerCloseCh
}

// getExec returns exec stream request
func (s *Server) getExec(req *pb.ExecRequest) (*pb.ExecResponse, error) {
	return s.stream.streamServer.GetExec(req)
}

// getAttach returns attach stream request
func (s *Server) getAttach(req *pb.AttachRequest) (*pb.AttachResponse, error) {
	return s.stream.streamServer.GetAttach(req)
}

// getPortForward returns port forward stream request
func (s *Server) getPortForward(req *pb.PortForwardRequest) (*pb.PortForwardResponse, error) {
	return s.stream.streamServer.GetPortForward(req)
}

func (s *Server) restore() {
	containers, err := s.Store().Containers()
	if err != nil && !os.IsNotExist(errors.Cause(err)) {
		logrus.Warnf("could not read containers and sandboxes: %v", err)
	}
	pods := map[string]*storage.RuntimeContainerMetadata{}
	podContainers := map[string]*storage.RuntimeContainerMetadata{}
	names := map[string][]string{}
	for i := range containers {
		container := &containers[i]
		metadata, err2 := s.StorageRuntimeServer().GetContainerMetadata(container.ID)
		if err2 != nil {
			logrus.Warnf("error parsing metadata for %s: %v, ignoring", container.ID, err2)
			continue
		}
		names[container.ID] = container.Names
		if metadata.Pod {
			pods[container.ID] = &metadata
		} else {
			podContainers[container.ID] = &metadata
		}
	}
	for containerID, metadata := range pods {
		if err = s.LoadSandbox(containerID); err != nil {
			logrus.Warnf("could not restore sandbox %s container %s: %v", metadata.PodID, containerID, err)
			for _, n := range names[containerID] {
				s.Store().DeleteContainer(n)
			}
		}
	}
	for containerID := range podContainers {
		if err := s.LoadContainer(containerID); err != nil {
			logrus.Warnf("could not restore container %s: %v", containerID, err)
			for _, n := range names[containerID] {
				s.Store().DeleteContainer(n)
			}
		}
	}
	// Restore sandbox IPs
	for _, sb := range s.ListSandboxes() {
		ip, err := s.getSandboxIP(sb)
		if err != nil {
			logrus.Warnf("could not restore sandbox IP for %v: %v", sb.ID(), err)
		}
		sb.AddIP(ip)
	}
}

// cleanupSandboxesOnShutdown Remove all running Sandboxes on system shutdown
func (s *Server) cleanupSandboxesOnShutdown(ctx context.Context) {
	_, err := os.Stat(shutdownFile)
	if err == nil || !os.IsNotExist(err) {
		logrus.Debugf("shutting down all sandboxes, on shutdown")
		s.stopAllPodSandboxes(ctx)
		err = os.Remove(shutdownFile)
		if err != nil {
			logrus.Warnf("Failed to remove %q", shutdownFile)
		}

	}
}

// Shutdown attempts to shut down the server's storage cleanly
func (s *Server) Shutdown(ctx context.Context) error {
	// why do this on clean shutdown! we want containers left running when crio
	// is down for whatever reason no?!
	// notice this won't trigger just on system halt but also on normal
	// crio.service restart!!!
	s.cleanupSandboxesOnShutdown(ctx)
	return s.ContainerServer.Shutdown()
}

// configureMaxThreads sets the Go runtime max threads threshold
// which is 90% of the kernel setting from /proc/sys/kernel/threads-max
func configureMaxThreads() error {
	mt, err := ioutil.ReadFile("/proc/sys/kernel/threads-max")
	if err != nil {
		return err
	}
	mtint, err := strconv.Atoi(strings.TrimSpace(string(mt)))
	if err != nil {
		return err
	}
	maxThreads := (mtint / 100) * 90
	debug.SetMaxThreads(maxThreads)
	logrus.Debugf("Golang's threads limit set to %d", maxThreads)
	return nil
}

func getIDMappings(config *Config) (*idtools.IDMappings, error) {
	if config.UIDMappings == "" || config.GIDMappings == "" {
		return nil, nil
	}

	parsedUIDsMappings, err := idtools.ParseIDMap(strings.Split(config.UIDMappings, ","), "UID")
	if err != nil {
		return nil, err
	}
	parsedGIDsMappings, err := idtools.ParseIDMap(strings.Split(config.GIDMappings, ","), "GID")
	if err != nil {
		return nil, err
	}

	return idtools.NewIDMappingsFromMaps(parsedUIDsMappings, parsedGIDsMappings), nil
}

// New creates a new Server with options provided
func New(ctx context.Context, configIface ConfigIface) (*Server, error) {
	if configIface == nil || configIface.GetData() == nil {
		return nil, fmt.Errorf("provided configuration interface or its data is nil")
	}
	config := configIface.GetData()

	if err := os.MkdirAll(config.ContainerAttachSocketDir, 0755); err != nil {
		return nil, err
	}

	// This is used to monitor container exits using inotify
	if err := os.MkdirAll(config.ContainerExitsDir, 0755); err != nil {
		return nil, err
	}
	containerServer, err := lib.New(ctx, configIface.GetLibConfigIface())
	if err != nil {
		return nil, err
	}

	netPlugin, err := ocicni.InitCNI(config.DefaultNetwork, config.NetworkDir, config.PluginDir...)
	if err != nil {
		return nil, err
	}
	iptInterface := utiliptables.New(utilexec.New(), utildbus.New(), utiliptables.ProtocolIpv4)
	iptInterface.EnsureChain(utiliptables.TableNAT, iptablesproxy.KubeMarkMasqChain)
	hostportManager := hostport.NewHostportManager(iptInterface)

	idMappings, err := getIDMappings(config)
	if err != nil {
		return nil, err
	}

	s := &Server{
		ContainerServer:   containerServer,
		netPlugin:         netPlugin,
		hostportManager:   hostportManager,
		config:            *config,
		seccompEnabled:    seccomp.IsEnabled(),
		appArmorEnabled:   apparmor.IsEnabled(),
		appArmorProfile:   config.ApparmorProfile,
		monitorsChan:      make(chan struct{}),
		defaultIDMappings: idMappings,
	}

	if s.seccompEnabled {
		seccompProfile, fileErr := ioutil.ReadFile(config.SeccompProfile)
		if fileErr != nil {
			return nil, fmt.Errorf("opening seccomp profile (%s) failed: %v", config.SeccompProfile, fileErr)
		}
		var seccompConfig seccomp.Seccomp
		if jsonErr := json.Unmarshal(seccompProfile, &seccompConfig); jsonErr != nil {
			return nil, fmt.Errorf("decoding seccomp profile failed: %v", jsonErr)
		}
		s.seccompProfile = seccompConfig
	}

	if s.appArmorEnabled && s.appArmorProfile == apparmorDefaultProfile {
		if err := apparmor.InstallDefault(apparmorDefaultProfile); err != nil {
			return nil, fmt.Errorf("ensuring the default apparmor profile %q is installed failed: %v", apparmorDefaultProfile, err)
		}
	}

	if err := configureMaxThreads(); err != nil {
		return nil, err
	}

	s.restore()
	s.cleanupSandboxesOnShutdown(ctx)

	hostIP := net.ParseIP(config.HostIP)
	if hostIP == nil {
		hostIP, err = knet.ChooseBindAddress(nil)
		if err != nil {
			return nil, err
		}
	}
	bindAddress := net.ParseIP(config.StreamAddress)
	if bindAddress == nil {
		bindAddress = hostIP
	}
	s.bindAddress = bindAddress.String()
	s.hostIP = hostIP.String()

	_, err = net.LookupPort("tcp", config.StreamPort)
	if err != nil {
		return nil, err
	}

	// Prepare streaming server
	streamServerConfig := streaming.DefaultConfig
	streamServerConfig.Addr = net.JoinHostPort(bindAddress.String(), config.StreamPort)
	if config.StreamEnableTLS {
		certCache := &certConfigCache{
			tlsCert: config.StreamTLSCert,
			tlsKey:  config.StreamTLSKey,
			tlsCA:   config.StreamTLSCA,
		}
		// We add the certs to the config, even thought the config is dynamic, because
		// the http package method, ServeTLS, checks to make sure there is a cert in the
		// config or it throws an error.
		cert, err := tls.LoadX509KeyPair(config.StreamTLSCert, config.StreamTLSKey)
		if err != nil {
			return nil, err
		}
		streamServerConfig.TLSConfig = &tls.Config{
			GetConfigForClient: certCache.GetConfigForClient,
			Certificates:       []tls.Certificate{cert},
		}
	}
	s.stream.runtimeServer = s
	s.stream.streamServer, err = streaming.NewServer(streamServerConfig, s.stream)
	if err != nil {
		return nil, fmt.Errorf("unable to create streaming server")
	}

	s.stream.streamServerCloseCh = make(chan struct{})
	go func() {
		defer close(s.stream.streamServerCloseCh)
		if err := s.stream.streamServer.Start(true); err != nil && err != http.ErrServerClosed {
			logrus.Errorf("Failed to start streaming server: %v", err)
		}
	}()

	logrus.Debugf("sandboxes: %v", s.ContainerServer.ListSandboxes())
	return s, nil
}

func (s *Server) addSandbox(sb *sandbox.Sandbox) error {
	return s.ContainerServer.AddSandbox(sb)
}

func (s *Server) getSandbox(id string) *sandbox.Sandbox {
	return s.ContainerServer.GetSandbox(id)
}

func (s *Server) removeSandbox(id string) error {
	return s.ContainerServer.RemoveSandbox(id)
}

func (s *Server) addContainer(c *oci.Container) {
	s.ContainerServer.AddContainer(c)
}

func (s *Server) addInfraContainer(c *oci.Container) {
	s.ContainerServer.AddInfraContainer(c)
}

func (s *Server) getInfraContainer(id string) *oci.Container {
	return s.ContainerServer.GetInfraContainer(id)
}

func (s *Server) removeContainer(c *oci.Container) {
	s.ContainerServer.RemoveContainer(c)
}

func (s *Server) removeInfraContainer(c *oci.Container) {
	s.ContainerServer.RemoveInfraContainer(c)
}

func (s *Server) getPodSandboxFromRequest(podSandboxID string) (*sandbox.Sandbox, error) {
	if podSandboxID == "" {
		return nil, sandbox.ErrIDEmpty
	}

	sandboxID, err := s.PodIDIndex().Get(podSandboxID)
	if err != nil {
		return nil, fmt.Errorf("PodSandbox with ID starting with %s not found: %v", podSandboxID, err)
	}

	sb := s.getSandbox(sandboxID)
	if sb == nil {
		return nil, fmt.Errorf("specified pod sandbox not found: %s", sandboxID)
	}
	return sb, nil
}

// CreateMetricsEndpoint creates a /metrics endpoint
// for prometheus monitoring
func (s *Server) CreateMetricsEndpoint() (*http.ServeMux, error) {
	metrics.Register()
	mux := &http.ServeMux{}
	mux.Handle("/metrics", promhttp.Handler())
	return mux, nil
}

// StopMonitors stops al the monitors
func (s *Server) StopMonitors() {
	close(s.monitorsChan)
}

// MonitorsCloseChan returns the close chan for the exit monitor
func (s *Server) MonitorsCloseChan() chan struct{} {
	return s.monitorsChan
}

// StartExitMonitor start a routine that monitors container exits
// and updates the container status
func (s *Server) StartExitMonitor() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logrus.Fatalf("Failed to create new watch: %v", err)
	}
	defer watcher.Close()

	done := make(chan struct{})
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				logrus.Debugf("event: %v", event)
				if event.Op&fsnotify.Create == fsnotify.Create {
					containerID := filepath.Base(event.Name)
					logrus.Debugf("container or sandbox exited: %v", containerID)
					c := s.GetContainer(containerID)
					if c != nil {
						logrus.Debugf("container exited and found: %v", containerID)
						err := s.Runtime().UpdateContainerStatus(c)
						if err != nil {
							logrus.Warnf("Failed to update container status %s: %v", containerID, err)
						} else {
							s.ContainerStateToDisk(c)
						}
					} else {
						sb := s.GetSandbox(containerID)
						if sb != nil {
							c := sb.InfraContainer()
							if c == nil {
								logrus.Warnf("no infra container set for sandbox: %v", containerID)
								continue
							}
							logrus.Debugf("sandbox exited and found: %v", containerID)
							err := s.Runtime().UpdateContainerStatus(c)
							if err != nil {
								logrus.Warnf("Failed to update sandbox infra container status %s: %v", c.ID(), err)
							} else {
								s.ContainerStateToDisk(c)
							}
						}
					}
				}
			case err := <-watcher.Errors:
				logrus.Debugf("watch error: %v", err)
				return
			case <-s.monitorsChan:
				logrus.Debug("closing exit monitor...")
				close(done)
				return
			}
		}
	}()
	if err := watcher.Add(s.config.ContainerExitsDir); err != nil {
		logrus.Errorf("watcher.Add(%q) failed: %s", s.config.ContainerExitsDir, err)
		close(done)
	}
	<-done
}
