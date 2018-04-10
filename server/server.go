package server

import (
	"context"
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

	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/fsnotify/fsnotify"
	"github.com/kubernetes-incubator/cri-o/lib"
	"github.com/kubernetes-incubator/cri-o/lib/sandbox"
	"github.com/kubernetes-incubator/cri-o/oci"
	"github.com/kubernetes-incubator/cri-o/pkg/storage"
	"github.com/kubernetes-incubator/cri-o/server/apparmor"
	"github.com/kubernetes-incubator/cri-o/server/metrics"
	"github.com/kubernetes-incubator/cri-o/server/seccomp"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	knet "k8s.io/apimachinery/pkg/util/net"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/runtime/v1alpha2"
	"k8s.io/kubernetes/pkg/kubelet/network/hostport"
	"k8s.io/kubernetes/pkg/kubelet/server/streaming"
	iptablesproxy "k8s.io/kubernetes/pkg/proxy/iptables"
	utildbus "k8s.io/kubernetes/pkg/util/dbus"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	utilexec "k8s.io/utils/exec"
)

const (
	shutdownFile = "/var/lib/crio/crio.shutdown"
)

func isTrue(annotaton string) bool {
	return annotaton == "true"
}

// streamService implements streaming.Runtime.
type streamService struct {
	runtimeServer       *Server // needed by Exec() endpoint
	streamServer        streaming.Server
	streamServerCloseCh chan struct{}
	streaming.Runtime
}

// Server implements the RuntimeService and ImageService
type Server struct {
	*lib.ContainerServer
	config Config

	updateLock      sync.RWMutex
	netPlugin       ocicni.CNIPlugin
	hostportManager hostport.HostPortManager

	seccompEnabled bool
	seccompProfile seccomp.Seccomp

	appArmorEnabled bool
	appArmorProfile string

	bindAddress  string
	stream       streamService
	monitorsChan chan struct{}
}

// StopStreamServer stops the stream server
func (s *Server) StopStreamServer() error {
	return s.stream.streamServer.Stop()
}

// StreamingServerCloseChan returns the close channel for the streaming server
func (s *Server) StreamingServerCloseChan() chan struct{} {
	return s.stream.streamServerCloseCh
}

// GetExec returns exec stream request
func (s *Server) GetExec(req *pb.ExecRequest) (*pb.ExecResponse, error) {
	return s.stream.streamServer.GetExec(req)
}

// GetAttach returns attach stream request
func (s *Server) GetAttach(req *pb.AttachRequest) (*pb.AttachResponse, error) {
	return s.stream.streamServer.GetAttach(req)
}

// GetPortForward returns port forward stream request
func (s *Server) GetPortForward(req *pb.PortForwardRequest) (*pb.PortForwardResponse, error) {
	return s.stream.streamServer.GetPortForward(req)
}

func (s *Server) restore() {
	containers, err := s.Store().Containers()
	if err != nil && !os.IsNotExist(errors.Cause(err)) {
		logrus.Warnf("could not read containers and sandboxes: %v", err)
	}
	pods := map[string]*storage.RuntimeContainerMetadata{}
	podContainers := map[string]*storage.RuntimeContainerMetadata{}
	for _, container := range containers {
		metadata, err2 := s.StorageRuntimeServer().GetContainerMetadata(container.ID)
		if err2 != nil {
			logrus.Warnf("error parsing metadata for %s: %v, ignoring", container.ID, err2)
			continue
		}
		if metadata.Pod {
			pods[container.ID] = &metadata
		} else {
			podContainers[container.ID] = &metadata
		}
	}
	for containerID, metadata := range pods {
		if err = s.LoadSandbox(containerID); err != nil {
			logrus.Warnf("could not restore sandbox %s container %s: %v", metadata.PodID, containerID, err)
		}
	}
	for containerID := range podContainers {
		if err := s.LoadContainer(containerID); err != nil {
			logrus.Warnf("could not restore container %s: %v", containerID, err)
		}
	}
	// Restore sandbox IPs
	for _, sb := range s.ListSandboxes() {
		ip, err := s.GetSandboxIP(sb)
		if err != nil {
			logrus.Warnf("could not restore sandbox IP for %v: %v", sb.ID(), err)
		}
		sb.AddIP(ip)
	}
}

// Update makes changes to the server's state (lists of pods and containers) to
// reflect the list of pods and containers that are stored on disk, possibly
// having been modified by other parties
func (s *Server) Update() {
	logrus.Debugf("updating sandbox and container information")
	if err := s.ContainerServer.Update(); err != nil {
		logrus.Errorf("error updating sandbox and container information: %v", err)
	}
}

// cleanupSandboxesOnShutdown Remove all running Sandboxes on system shutdown
func (s *Server) cleanupSandboxesOnShutdown(ctx context.Context) {
	_, err := os.Stat(shutdownFile)
	if err == nil || !os.IsNotExist(err) {
		logrus.Debugf("shutting down all sandboxes, on shutdown")
		s.StopAllPodSandboxes(ctx)
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

// New creates a new Server with options provided
func New(ctx context.Context, config *Config) (*Server, error) {
	if err := os.MkdirAll("/var/run/crio", 0755); err != nil {
		return nil, err
	}

	config.ContainerExitsDir = oci.ContainerExitsDir

	// This is used to monitor container exits using inotify
	if err := os.MkdirAll(config.ContainerExitsDir, 0755); err != nil {
		return nil, err
	}
	containerServer, err := lib.New(&config.Config)
	if err != nil {
		return nil, err
	}

	netPlugin, err := ocicni.InitCNI(config.NetworkDir, config.PluginDir)
	if err != nil {
		return nil, err
	}
	iptInterface := utiliptables.New(utilexec.New(), utildbus.New(), utiliptables.ProtocolIpv4)
	iptInterface.EnsureChain(utiliptables.TableNAT, iptablesproxy.KubeMarkMasqChain)
	hostportManager := hostport.NewHostportManager(iptInterface)

	s := &Server{
		ContainerServer: containerServer,
		netPlugin:       netPlugin,
		hostportManager: hostportManager,
		config:          *config,
		seccompEnabled:  seccomp.IsEnabled(),
		appArmorEnabled: apparmor.IsEnabled(),
		appArmorProfile: config.ApparmorProfile,
		monitorsChan:    make(chan struct{}),
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

	if s.appArmorEnabled && s.appArmorProfile == apparmor.DefaultApparmorProfile {
		if apparmorErr := apparmor.EnsureDefaultApparmorProfile(); apparmorErr != nil {
			return nil, fmt.Errorf("ensuring the default apparmor profile is installed failed: %v", apparmorErr)
		}
	}

	if err := configureMaxThreads(); err != nil {
		return nil, err
	}

	s.restore()
	s.cleanupSandboxesOnShutdown(ctx)

	bindAddress := net.ParseIP(config.StreamAddress)
	if bindAddress == nil {
		bindAddress, err = knet.ChooseBindAddress(net.IP{0, 0, 0, 0})
		if err != nil {
			return nil, err
		}
	}
	s.bindAddress = bindAddress.String()

	_, err = net.LookupPort("tcp", config.StreamPort)
	if err != nil {
		return nil, err
	}

	// Prepare streaming server
	streamServerConfig := streaming.DefaultConfig
	streamServerConfig.Addr = net.JoinHostPort(bindAddress.String(), config.StreamPort)
	s.stream.runtimeServer = s
	s.stream.streamServer, err = streaming.NewServer(streamServerConfig, s.stream)
	if err != nil {
		return nil, fmt.Errorf("unable to create streaming server")
	}

	s.stream.streamServerCloseCh = make(chan struct{})
	go func() {
		defer close(s.stream.streamServerCloseCh)
		if err := s.stream.streamServer.Start(true); err != nil {
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

func (s *Server) hasSandbox(id string) bool {
	return s.ContainerServer.HasSandbox(id)
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

func (s *Server) getContainer(id string) *oci.Container {
	return s.ContainerServer.GetContainer(id)
}

func (s *Server) getInfraContainer(id string) *oci.Container {
	return s.ContainerServer.GetInfraContainer(id)
}

// BindAddress is used to retrieve host's IP
func (s *Server) BindAddress() string {
	return s.bindAddress
}

// GetSandboxContainer returns the infra container for a given sandbox
func (s *Server) GetSandboxContainer(id string) *oci.Container {
	return s.ContainerServer.GetSandboxContainer(id)
}

// GetContainer returns a container by its ID
func (s *Server) GetContainer(id string) *oci.Container {
	return s.getContainer(id)
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
	mux.Handle("/metrics", prometheus.Handler())
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

// StartHooksMonitor starts a goroutine to dynamically add hooks at runtime
func (s *Server) StartHooksMonitor() {
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
				filename := filepath.Base(event.Name)
				overridePath := filepath.Join(lib.OverrideHooksDirPath, filename)
				overriddenPath := filepath.Join(s.config.HooksDirPath, filename)
				override := event.Name == overridePath
				if event.Op&fsnotify.Remove == fsnotify.Remove {
					if override {
						err = s.ContainerServer.AddHook(overriddenPath)
						if err == nil {
							logrus.Debugf("override %q removed, falling back to %q", event.Name, overriddenPath)
							break
						} else if err != lib.ErrNoJSONSuffix && !os.IsNotExist(err) {
							logrus.Errorf("failed to fall back to %q: %v", overriddenPath, err)
							break
						}
					}
					ok := s.ContainerServer.RemoveHook(filename)
					if ok {
						logrus.Debugf("removed hook %q", event.Name)
					}
				}
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Write == fsnotify.Write {
					if !override && s.config.HooksDirPath == lib.DefaultHooksDirPath {
						_, err = os.Lstat(overridePath)
						if err == nil {
							logrus.Errorf("%q updated, but it is overridden by %q", event.Name, overridePath)
							break // the touched path is being overridden
						} else if !os.IsNotExist(err) {
							logrus.Errorf("failed to check for override %q: %v", overridePath, err)
						}
					}
					err := s.ContainerServer.AddHook(event.Name)
					if err == nil {
						logrus.Debugf("added hook %s", event.Name)
					} else if err != lib.ErrNoJSONSuffix {
						logrus.Errorf("failed to add hook %s: %v", event.Name, err)
					}
				}
			case err := <-watcher.Errors:
				logrus.Debugf("watch error: %v", err)
				return
			case <-s.monitorsChan:
				logrus.Debug("closing hooks monitor...")
				close(done)
				return
			}
		}
	}()
	// If user overrode default hooks, this means it is in a test, so
	// don't use OverrideHooksDirPath.  Sync with lib/container_server.go's
	// New.
	if s.config.HooksDirPath == lib.DefaultHooksDirPath {
		if err := watcher.Add(lib.OverrideHooksDirPath); err != nil {
			logrus.Errorf("watcher.Add(%q) failed: %s", lib.OverrideHooksDirPath, err)
			close(done)
			return
		}
	}
	if err := watcher.Add(s.config.HooksDirPath); err != nil {
		logrus.Errorf("watcher.Add(%q) failed: %s", s.config.HooksDirPath, err)
		close(done)
	}
	<-done
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
						err := s.Runtime().UpdateStatus(c)
						if err != nil {
							logrus.Warnf("Failed to update container status %s: %v", c, err)
						} else {
							s.ContainerStateToDisk(c)
						}
					} else {
						sb := s.GetSandbox(containerID)
						if sb != nil {
							c := sb.InfraContainer()
							logrus.Debugf("sandbox exited and found: %v", containerID)
							err := s.Runtime().UpdateStatus(c)
							if err != nil {
								logrus.Warnf("Failed to update sandbox infra container status %s: %v", c, err)
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
