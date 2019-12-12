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
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
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

// streamService implements streaming.Runtime.
type streamService struct {
	runtimeServer       *Server // needed by Exec() endpoint
	streamServer        streaming.Server
	streamServerCloseCh chan struct{}
	streaming.Runtime
}

// Server implements the RuntimeService and ImageService
type Server struct {
	config          Config
	seccompProfile  seccomp.Seccomp
	stream          streamService
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
	deletedPods := map[string]bool{}
	for i := range containers {
		metadata, err2 := s.StorageRuntimeServer().GetContainerMetadata(containers[i].ID)
		if err2 != nil {
			logrus.Warnf("error parsing metadata for %s: %v, ignoring", containers[i].ID, err2)
			continue
		}
		if !storage.IsCrioContainer(metadata) {
			logrus.Debugf("container %s determined to not be a CRI-O container or sandbox", containers[i].ID)
			continue
		}
		names[containers[i].ID] = containers[i].Names
		if metadata.Pod {
			pods[containers[i].ID] = &metadata
		} else {
			podContainers[containers[i].ID] = &metadata
		}
	}

	// Go through all the pods and check if it can be restored. If an error occurs, delete the pod and any containers
	// associated with it. Release the pod and container names as well.
	for sbID, metadata := range pods {
		if err = s.LoadSandbox(sbID); err == nil {
			continue
		}
		logrus.Warnf("could not restore sandbox %s container %s: %v", metadata.PodID, sbID, err)
		for _, n := range names[sbID] {
			s.Store().DeleteContainer(n)
			// Release the infra container name and the pod name for future use
			if strings.Contains(n, infraName) {
				s.ReleaseContainerName(n)
			} else {
				s.ReleasePodName(n)
			}

		}
		// Go through the containers and delete any containers that were under the deleted pod
		logrus.Warnf("deleting all containers under sandbox %s since it could not be restored", sbID)
		for k, v := range podContainers {
			if v.PodID == sbID {
				for _, n := range names[k] {
					s.Store().DeleteContainer(n)
					// Release the container name for future use
					s.ReleaseContainerName(n)
				}
			}
		}
		// Add the pod id to the list of deletedPods so we don't try to restore IPs for it later on
		deletedPods[sbID] = true
	}

	// Go through all the containers and check if it can be restored. If an error occurs, delete the container and
	// release the name associated with it.
	for containerID := range podContainers {
		if err := s.LoadContainer(containerID); err != nil {
			logrus.Warnf("could not restore container %s: %v", containerID, err)
			for _, n := range names[containerID] {
				s.Store().DeleteContainer(n)
				// Release the container name
				s.ReleaseContainerName(n)
			}
		}
	}

	// Restore sandbox IPs
	for _, sb := range s.ListSandboxes() {
		// Clean up networking if pod couldn't be restored and was deleted
		if ok := deletedPods[sb.ID()]; ok {
			s.networkStop(sb)
			continue
		}
		ip, err := s.getSandboxIP(sb)
		if err != nil {
			logrus.Warnf("could not restore sandbox IP for %v: %v", sb.ID(), err)
		}
		sb.AddIP(ip)
	}

	// Clean up orphaned exit files
	var exitIDs []string

	exitDir := s.Config().RuntimeConfig.ContainerExitsDir
	err = filepath.Walk(exitDir, func(path string, info os.FileInfo, err error) error {
		exitFileName := filepath.Base(path)
		if path != exitDir {
			exitIDs = append(exitIDs, exitFileName)
		}
		return nil
	})
	if err != nil {
		logrus.Warnf("Failed to walk exit dir %v: %v", exitDir, err)
	}
	for _, exitID := range exitIDs {
		logrus.Warnf("Checking exit file: %v", exitID)
		ctr := s.GetContainer(exitID)
		if ctr != nil {
			continue
		} else {
			sb := s.GetSandbox(exitID)
			if sb != nil {
				continue
			}
		}
		logrus.Warnf("Removing exit file: %v", exitID)
		if err := os.Remove(filepath.Join(exitDir, exitID)); err != nil && !os.IsNotExist(err) {
			logrus.Warnf("Failed to remove container exit file during restore cleanup %s: %v", exitID, err)
		}
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

	s.ShutdownConmonmon()
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

	parseTriple := func(spec []string) (container, host, size int, err error) {
		cid, err := strconv.ParseUint(spec[0], 10, 32)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("error parsing id map value %q: %v", spec[0], err)
		}
		hid, err := strconv.ParseUint(spec[1], 10, 32)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("error parsing id map value %q: %v", spec[1], err)
		}
		sz, err := strconv.ParseUint(spec[2], 10, 32)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("error parsing id map value %q: %v", spec[2], err)
		}
		return int(cid), int(hid), int(sz), nil
	}
	parseIDMap := func(spec []string) (idmap []idtools.IDMap, err error) {
		for _, uid := range spec {
			splitmap := strings.SplitN(uid, ":", 3)
			if len(splitmap) < 3 {
				return nil, fmt.Errorf("invalid mapping requires 3 fields: %q", uid)
			}
			cid, hid, size, err := parseTriple(splitmap)
			if err != nil {
				return nil, err
			}
			pmap := idtools.IDMap{
				ContainerID: cid,
				HostID:      hid,
				Size:        size,
			}
			idmap = append(idmap, pmap)
		}
		return idmap, nil
	}

	parsedUIDsMappings, err := parseIDMap(strings.Split(config.UIDMappings, ","))
	if err != nil {
		return nil, err
	}
	parsedGIDsMappings, err := parseIDMap(strings.Split(config.GIDMappings, ","))
	if err != nil {
		return nil, err
	}

	return idtools.NewIDMappingsFromMaps(parsedUIDsMappings, parsedGIDsMappings), nil
}

// New creates a new Server with options provided
func New(ctx context.Context, config *Config) (*Server, error) {
	if err := os.MkdirAll(oci.ContainerAttachSocketDir, 0755); err != nil {
		return nil, err
	}

	// This is used to monitor container exits using inotify
	if err := os.MkdirAll(config.ContainerExitsDir, 0755); err != nil {
		return nil, err
	}
	containerServer, err := lib.New(ctx, &config.Config)
	if err != nil {
		return nil, err
	}

	netPlugin, err := ocicni.InitCNI("", config.NetworkDir, config.PluginDirs...)
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
		// First, attempt to find a primary IP from /proc/net/route deterministically
		hostIP, err = knet.ChooseBindAddress(nil)
		if err != nil {
			// if that fails, check if we can find a primary IP address unambiguously
			allAddrs, err2 := net.InterfaceAddrs()
			if err2 != nil {
				return nil, errors.Wrapf(err, "Failed to read InterfaceAddrs after failing to choose bind address")
			}

			// we are hoping for exactly 1 possible IP
			numPossibleIPs := 0
			// adapted from: https://stackoverflow.com/a/31551220
			for _, addr := range allAddrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						numPossibleIPs++
						hostIP = ipnet.IP
						if numPossibleIPs > 1 {
							break
						}
					}
				}
			}

			// If there is no clear primary IP, we should fail
			if numPossibleIPs != 1 {
				return nil, errors.Wrapf(err, "Failed to find one IP address after failing to choose bind address")
			}
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

func (s *Server) addSandbox(sb *sandbox.Sandbox) {
	s.ContainerServer.AddSandbox(sb)
}

func (s *Server) getSandbox(id string) *sandbox.Sandbox {
	return s.ContainerServer.GetSandbox(id)
}

func (s *Server) removeSandbox(id string) {
	s.ContainerServer.RemoveSandbox(id)
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
