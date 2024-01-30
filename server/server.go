package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	imageTypes "github.com/containers/image/v5/types"
	"github.com/containers/storage/pkg/idtools"
	storageTypes "github.com/containers/storage/types"
	"github.com/cri-o/cri-o/internal/config/seccomp"
	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/resourcestore"
	"github.com/cri-o/cri-o/internal/runtimehandlerhooks"
	"github.com/cri-o/cri-o/internal/signals"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/version"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/server/metrics"
	"github.com/cri-o/cri-o/utils"
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/kubelet/pkg/cri/streaming"
	kubetypes "k8s.io/kubelet/pkg/types"

	nriIf "github.com/cri-o/cri-o/internal/nri"
)

const (
	certRefreshInterval            = time.Minute * 5
	rootlessEnvName                = "_CRIO_ROOTLESS"
	irqBalanceConfigRestoreDisable = "disable"
)

var errSandboxNotCreated = errors.New("sandbox not created")

// StreamService implements streaming.Runtime.
type StreamService struct {
	ctx                 context.Context
	runtimeServer       *Server // needed by Exec() endpoint
	streamServer        streaming.Server
	streamServerCloseCh chan struct{}
	streaming.Runtime
}

// Server implements the RuntimeService and ImageService
type Server struct {
	config          libconfig.Config
	stream          StreamService
	hostportManager hostport.HostPortManager

	*lib.ContainerServer
	monitorsChan        chan struct{}
	defaultIDMappings   *idtools.IDMappings
	ContainerEventsChan chan types.ContainerEventResponse

	minimumMappableUID, minimumMappableGID int64

	// pullOperationsInProgress is used to avoid pulling the same image in parallel. Goroutines
	// will block on the pullResult.
	pullOperationsInProgress map[pullArguments]*pullOperation
	// pullOperationsLock is used to synchronize pull operations.
	pullOperationsLock sync.Mutex

	resourceStore *resourcestore.ResourceStore

	seccompNotifierChan chan seccomp.Notification
	seccompNotifiers    sync.Map

	containerEventClients           sync.Map
	containerEventStreamBroadcaster sync.Once

	// NRI runtime interface
	nri *nriAPI
}

// pullArguments are used to identify a pullOperation via an input image name and
// possibly specified credentials.
type pullArguments struct {
	image         string
	sandboxCgroup string
	credentials   imageTypes.DockerAuthConfig
	namespace     string
}

// pullOperation is used to synchronize parallel pull operations via the
// server's pullCache.  Goroutines can block the pullOperation's waitgroup and
// be released once the pull operation has finished.
type pullOperation struct {
	// wg allows for Goroutines trying to pull the same image to wait until the
	// currently running pull operation has finished.
	wg sync.WaitGroup
	// imageRef is the reference of the actually pulled image which will differ
	// from the input if it was a short name (e.g., alpine).
	imageRef string
	// err is the error indicating if the pull operation has succeeded or not.
	err error
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
		caBytes, err := os.ReadFile(cc.tlsCA)
		if err != nil {
			return nil, fmt.Errorf("read TLS CA file: %w", err)
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
func (s *Server) getExec(req *types.ExecRequest) (*types.ExecResponse, error) {
	return s.stream.streamServer.GetExec(req)
}

// getAttach returns attach stream request
func (s *Server) getAttach(req *types.AttachRequest) (*types.AttachResponse, error) {
	return s.stream.streamServer.GetAttach(req)
}

// getPortForward returns port forward stream request
func (s *Server) getPortForward(req *types.PortForwardRequest) (*types.PortForwardResponse, error) {
	return s.stream.streamServer.GetPortForward(req)
}

// restore attempts to restore the sandboxes and containers.
// For every sandbox it fails to restore, it starts a cleanup routine attempting to call CNI DEL
// For every container it fails to restore, it returns that containers image, so that
// it can be cleaned up (if we're using internal_wipe).
func (s *Server) restore(ctx context.Context) []storage.StorageImageID {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	containersAndTheirImages := map[string]storage.StorageImageID{}
	containers, err := s.Store().Containers()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warnf(ctx, "Could not read containers and sandboxes: %v", err)
	}
	pods := map[string]*storage.RuntimeContainerMetadata{}
	podContainers := map[string]*storage.RuntimeContainerMetadata{}
	names := map[string][]string{}
	deletedPods := map[string]*sandbox.Sandbox{}
	for i := range containers {
		metadata, err2 := s.StorageRuntimeServer().GetContainerMetadata(containers[i].ID)
		if err2 != nil {
			log.Warnf(ctx, "Error parsing metadata for %s: %v, ignoring", containers[i].ID, err2)
			continue
		}
		if !storage.IsCrioContainer(&metadata) {
			log.Debugf(ctx, "Container %s determined to not be a CRI-O container or sandbox", containers[i].ID)
			continue
		}
		names[containers[i].ID] = containers[i].Names
		if metadata.Pod {
			pods[containers[i].ID] = &metadata
		} else {
			podContainers[containers[i].ID] = &metadata
			imageID, err := storage.ParseStorageImageIDFromOutOfProcessData(containers[i].ImageID)
			if err != nil {
				log.Warnf(ctx, "Error parsing image ID %q of container %q: %v, ignoring", containers[i].ImageID, containers[i].ID, err)
				continue
			}
			containersAndTheirImages[containers[i].ID] = imageID
		}
	}

	// Go through all the pods and check if it can be restored. If an error occurs, delete the pod and any containers
	// associated with it. Release the pod and container names as well.
	for sbID := range pods {
		sb, err := s.LoadSandbox(ctx, sbID)
		if err == nil {
			continue
		}
		log.Warnf(ctx, "Could not restore sandbox %s: %v", sbID, err)
		for _, n := range names[sbID] {
			if err := s.Store().DeleteContainer(n); err != nil && err != storageTypes.ErrNotAContainer {
				log.Warnf(ctx, "Unable to delete container %s: %v", n, err)
			}
			// Release the infra container name and the pod name for future use
			if strings.Contains(n, oci.InfraContainerName) {
				s.ReleaseContainerName(ctx, n)
			} else {
				s.ReleasePodName(n)
			}
		}
		// Go through the containers and delete any container that was under the deleted pod
		log.Warnf(ctx, "Deleting all containers under sandbox %s since it could not be restored", sbID)
		for k, v := range podContainers {
			if v.PodID != sbID {
				continue
			}
			for _, n := range names[k] {
				if err := s.Store().DeleteContainer(n); err != nil && err != storageTypes.ErrNotAContainer {
					log.Warnf(ctx, "Unable to delete container %s: %v", n, err)
				}
				// Release the container name for future use
				s.ReleaseContainerName(ctx, n)
			}
			// Remove the container from the list of podContainers, or else we'll retry the delete later,
			// causing a useless debug message.
			delete(podContainers, k)
		}
		// Add the pod id to the list of deletedPods, to be able to call CNI DEL on the sandbox network.
		// Unfortunately, if we weren't able to restore a sandbox, then there's little that can be done
		if sb != nil {
			deletedPods[sbID] = sb
		}
	}

	// Go through all the containers and check if it can be restored. If an error occurs, delete the container and
	// release the name associated with you.
	for containerID := range podContainers {
		err := s.LoadContainer(ctx, containerID)
		if err == nil || err == lib.ErrIsNonCrioContainer {
			delete(containersAndTheirImages, containerID)
			continue
		}
		log.Warnf(ctx, "Could not restore container %s: %v", containerID, err)
		for _, n := range names[containerID] {
			if err := s.Store().DeleteContainer(n); err != nil && err != storageTypes.ErrNotAContainer {
				log.Warnf(ctx, "Unable to delete container %s: %v", n, err)
			}
			// Release the container name
			s.ReleaseContainerName(ctx, n)
		}
	}

	// Cleanup the deletedPods in the networking plugin
	wipeResourceCleaner := resourcestore.NewResourceCleaner()
	for _, sb := range deletedPods {
		sb := sb
		cleanupFunc := func() error {
			err := s.networkStop(context.Background(), sb)
			if err == nil {
				log.Infof(ctx, "Successfully cleaned up network for pod %s", sb.ID())
			}
			return err
		}
		wipeResourceCleaner.Add(ctx, "cleanup sandbox network", cleanupFunc)
	}

	// If any failed to be deleted, the networking plugin is likely not ready.
	// The cleanup should be retried until it succeeds.
	go func() {
		if err := wipeResourceCleaner.Cleanup(); err != nil {
			log.Errorf(ctx, "Cleanup during server startup failed: %v", err)
		}
	}()

	// Restore sandbox IPs
	for _, sb := range s.ListSandboxes() {
		ips, err := s.getSandboxIPs(ctx, sb)
		if err != nil {
			log.Warnf(ctx, "Could not restore sandbox IP for %v: %v", sb.ID(), err)
			continue
		}
		sb.AddIPs(ips)
	}

	// Return a slice of images to remove, if internal_wipe is set.
	imagesOfDeletedContainers := []storage.StorageImageID{}
	for _, image := range containersAndTheirImages {
		imagesOfDeletedContainers = append(imagesOfDeletedContainers, image)
	}

	return imagesOfDeletedContainers
}

// Shutdown attempts to shut down the server's storage cleanly
func (s *Server) Shutdown(ctx context.Context) error {
	s.config.CNIManagerShutdown()
	s.resourceStore.Close()

	if err := s.ContainerServer.Shutdown(); err != nil {
		return err
	}

	// first, make sure we sync all the changes to the file system holding
	// the graph root
	if err := utils.Syncfs(s.Store().GraphRoot()); err != nil {
		return fmt.Errorf("failed to sync graph root after shutting down: %w", err)
	}

	if s.config.CleanShutdownFile != "" {
		// then, we write the CleanShutdownFile
		// we do this after the sync, to ensure ordering.
		// Otherwise, we may run into situations where the CleanShutdownFile
		// is written before storage, causing us to think a corrupted storage
		// is not so.
		f, err := os.Create(s.config.CleanShutdownFile)
		if err != nil {
			return fmt.Errorf("failed to write file to indicate a clean shutdown: %w", err)
		}
		f.Close()

		// finally, attempt to sync the newly created file to disk.
		// It's still possible we crash after Create but before this Sync,
		// which will lead us to think storage wasn't synced.
		// However, that's much less likely than if we don't have a second Sync,
		// and less risky than if we don't Sync after the Create
		if err := utils.SyncParent(s.config.CleanShutdownFile); err != nil {
			return fmt.Errorf("failed to sync clean shutdown file: %w", err)
		}
	}

	if s.config.EnablePodEvents {
		// closing a non-nil channel only if the evented pleg is enabled
		close(s.ContainerEventsChan)
	}

	return nil
}

func getIDMappings(config *libconfig.Config) (*idtools.IDMappings, error) {
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

// New creates a new Server with the provided context and configuration
func New(
	ctx context.Context,
	configIface libconfig.Iface,
) (*Server, error) {
	if configIface == nil || configIface.GetData() == nil {
		return nil, fmt.Errorf("provided configuration interface or its data is nil")
	}
	config := configIface.GetData()

	useDefaultUmask()

	config.SystemContext.AuthFilePath = config.GlobalAuthFile
	config.SystemContext.SignaturePolicyPath = config.SignaturePolicyPath

	if err := os.MkdirAll(config.ContainerAttachSocketDir, 0o755); err != nil {
		return nil, err
	}

	// This is used to monitor container exits using inotify
	if err := os.MkdirAll(config.ContainerExitsDir, 0o755); err != nil {
		return nil, err
	}
	containerServer, err := lib.New(ctx, configIface)
	if err != nil {
		return nil, err
	}

	if strings.ToLower(strings.TrimSpace(config.IrqBalanceConfigRestoreFile)) != irqBalanceConfigRestoreDisable {
		log.Infof(ctx, "Attempting to restore irqbalance config from %s", config.IrqBalanceConfigRestoreFile)
		err = runtimehandlerhooks.RestoreIrqBalanceConfig(context.TODO(), config.IrqBalanceConfigFile, config.IrqBalanceConfigRestoreFile, runtimehandlerhooks.IrqSmpAffinityProcFile)
		if err != nil {
			return nil, err
		}
	}

	// Check for hostport mapping
	var hostportManager hostport.HostPortManager
	if config.RuntimeConfig.DisableHostPortMapping {
		hostportManager = hostport.NewNoopHostportManager()
	} else {
		hostportManager = hostport.NewMetaHostportManager()
	}

	idMappings, err := getIDMappings(config)
	if err != nil {
		return nil, err
	}
	if idMappings != nil {
		log.Errorf(ctx, "Configuration options 'uid_mappings' and 'gid_mappings' are deprecated, and will be replaced with native Kubernetes support for user namespaces in the future")
	}

	if os.Getenv(rootlessEnvName) == "" {
		// Not running as rootless, reset XDG_RUNTIME_DIR and DBUS_SESSION_BUS_ADDRESS
		os.Unsetenv("XDG_RUNTIME_DIR")
		os.Unsetenv("DBUS_SESSION_BUS_ADDRESS")
	}

	s := &Server{
		ContainerServer:          containerServer,
		hostportManager:          hostportManager,
		config:                   *config,
		monitorsChan:             make(chan struct{}),
		defaultIDMappings:        idMappings,
		minimumMappableUID:       config.MinimumMappableUID,
		minimumMappableGID:       config.MinimumMappableGID,
		pullOperationsInProgress: make(map[pullArguments]*pullOperation),
		resourceStore:            resourcestore.New(),
	}
	if s.config.EnablePodEvents {
		// creating a container events channel only if the evented pleg is enabled
		s.ContainerEventsChan = make(chan types.ContainerEventResponse, 1000)
	}
	if err := configureMaxThreads(); err != nil {
		return nil, err
	}

	// Close stdin, so shortnames will not prompt
	devNullFile, err := os.Open(os.DevNull)
	if err != nil {
		return nil, fmt.Errorf("open devnull file: %w", err)
	}

	defer devNullFile.Close()
	if err := unix.Dup2(int(devNullFile.Fd()), int(os.Stdin.Fd())); err != nil {
		return nil, fmt.Errorf("close stdin: %w", err)
	}

	deletedImages := s.restore(ctx)
	s.wipeIfAppropriate(ctx, deletedImages)

	var bindAddressStr string
	bindAddress := net.ParseIP(config.StreamAddress)
	if bindAddress != nil {
		bindAddressStr = bindAddress.String()
	}

	_, err = net.LookupPort("tcp", config.StreamPort)
	if err != nil {
		return nil, err
	}

	// Prepare streaming server
	streamServerConfig := streaming.DefaultConfig
	if config.StreamIdleTimeout != "" {
		idleTimeout, err := time.ParseDuration(config.StreamIdleTimeout)
		if err != nil {
			return nil, errors.New("unable to parse timeout as duration")
		}

		streamServerConfig.StreamIdleTimeout = idleTimeout
	}
	streamServerConfig.Addr = net.JoinHostPort(bindAddressStr, config.StreamPort)
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
			MinVersion:         tls.VersionTLS12,
		}
	}
	s.stream.ctx = ctx
	s.stream.runtimeServer = s
	s.stream.streamServer, err = streaming.NewServer(streamServerConfig, s.stream)
	if err != nil {
		return nil, fmt.Errorf("unable to create streaming server")
	}

	s.stream.streamServerCloseCh = make(chan struct{})
	go func() {
		defer close(s.stream.streamServerCloseCh)
		if err := s.stream.streamServer.Start(true); err != nil && err != http.ErrServerClosed {
			log.Fatalf(ctx, "Failed to start streaming server: %v", err)
		}
	}()

	log.Debugf(ctx, "Sandboxes: %v", s.ContainerServer.ListSandboxes())

	s.startReloadWatcher(ctx)

	// Start the metrics server if configured to be enabled
	if s.config.EnableMetrics {
		if err := metrics.New(&s.config.MetricsConfig).Start(s.monitorsChan); err != nil {
			return nil, err
		}
	} else {
		logrus.Debug("Metrics are disabled")
	}

	if err := s.startSeccompNotifierWatcher(ctx); err != nil {
		return nil, fmt.Errorf("start seccomp notifier watcher: %w", err)
	}

	// Set up our NRI adaptation.
	api, err := nriIf.New(s.config.NRI.WithTracing(s.config.EnableTracing))
	if err != nil {
		return nil, fmt.Errorf("failed to create NRI interface: %v", err)
	}

	s.nri = &nriAPI{
		cri: s,
		nri: api,
	}

	if err := s.nri.start(); err != nil {
		return nil, err
	}

	return s, nil
}

// startReloadWatcher starts a new SIGHUP go routine.
func (s *Server) startReloadWatcher(ctx context.Context) {
	// Setup the signal notifier
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals.Hup)

	go func() {
		for {
			// Block until the signal is received
			<-ch
			if err := s.config.Reload(); err != nil {
				logrus.Errorf("Unable to reload configuration: %v", err)
				continue
			}
		}
	}()

	log.Infof(ctx, "Registered SIGHUP reload watcher")
}

func useDefaultUmask() {
	const defaultUmask = 0o022
	oldUmask := unix.Umask(defaultUmask)
	if oldUmask != defaultUmask {
		logrus.Infof(
			"Using default umask 0o%#o instead of 0o%#o",
			defaultUmask, oldUmask,
		)
	}
}

// wipeIfAppropriate takes a list of images. If the config's VersionFilePersist
// indicates an upgrade has happened, it attempts to wipe that list of images.
// This attempt is best-effort.
func (s *Server) wipeIfAppropriate(ctx context.Context, imagesToDelete []storage.StorageImageID) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if !s.config.InternalWipe {
		return
	}
	var (
		shouldWipeContainers, shouldWipeImages bool
		err                                    error
	)

	// Check if our persistent version file is out of date.
	// If so, we have upgrade, and we should wipe images.
	shouldWipeImages, err = version.ShouldCrioWipe(s.config.VersionFilePersist)
	if err != nil {
		log.Warnf(ctx, "Error encountered when checking whether cri-o should wipe images: %v", err)
	}

	// Unconditionally wipe containers if we should wipe images.
	if shouldWipeImages {
		shouldWipeContainers = true
	} else {
		// Check if our version file is out of date.
		// If so, we rebooted, and we should wipe containers.
		shouldWipeContainers, err = version.ShouldCrioWipe(s.config.VersionFile)
		if err != nil {
			log.Warnf(ctx, "Error encountered when checking whether cri-o should wipe containers: %v", err)
		}
	}

	// Translate to a map so the images are only attempted to be deleted once.
	imageMapToDelete := make(map[storage.StorageImageID]struct{})
	for _, img := range imagesToDelete {
		imageMapToDelete[img] = struct{}{}
	}

	// Attempt to wipe containers, adding the images that were removed on the way.
	if shouldWipeContainers {
		// Best-effort append to imageMapToDelete
		if ctrs, err := s.ContainerServer.ListContainers(); err == nil {
			for _, ctr := range ctrs {
				if id := ctr.ImageID(); id != nil {
					imageMapToDelete[*id] = struct{}{}
				}
			}
		}
		for _, sb := range s.ContainerServer.ListSandboxes() {
			if err := s.removePodSandbox(ctx, sb); err != nil {
				log.Warnf(ctx, "Failed to remove sandbox %s: %v", sb.ID(), err)
			}
		}
	}

	// Note: some of these will fail if some aspect of the pod cleanup failed as well,
	// but this is best-effort anyway, as the Kubelet will eventually cleanup images when
	// disk usage gets too high.
	if shouldWipeImages {
		for img := range imageMapToDelete {
			if err := s.StorageImageServer().DeleteImage(s.config.SystemContext, img); err != nil {
				log.Warnf(ctx, "Failed to remove image %s: %v", img, err)
			}
		}
	}
}

func (s *Server) addSandbox(ctx context.Context, sb *sandbox.Sandbox) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	return s.ContainerServer.AddSandbox(ctx, sb)
}

func (s *Server) getSandbox(ctx context.Context, id string) *sandbox.Sandbox {
	_, span := log.StartSpan(ctx)
	defer span.End()
	return s.ContainerServer.GetSandbox(id)
}

func (s *Server) removeSandbox(ctx context.Context, id string) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	return s.ContainerServer.RemoveSandbox(ctx, id)
}

func (s *Server) addContainer(ctx context.Context, c *oci.Container) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	s.ContainerServer.AddContainer(ctx, c)
}

func (s *Server) addInfraContainer(ctx context.Context, c *oci.Container) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	s.ContainerServer.AddInfraContainer(ctx, c)
}

func (s *Server) getInfraContainer(ctx context.Context, id string) *oci.Container {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	return s.ContainerServer.GetInfraContainer(ctx, id)
}

func (s *Server) removeContainer(ctx context.Context, c *oci.Container) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	s.ContainerServer.RemoveContainer(ctx, c)
}

func (s *Server) removeInfraContainer(ctx context.Context, c *oci.Container) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	s.ContainerServer.RemoveInfraContainer(ctx, c)
}

func (s *Server) getPodSandboxFromRequest(ctx context.Context, podSandboxID string) (*sandbox.Sandbox, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	if podSandboxID == "" {
		return nil, sandbox.ErrIDEmpty
	}

	sandboxID, err := s.PodIDIndex().Get(podSandboxID)
	if err != nil {
		return nil, fmt.Errorf("PodSandbox with ID starting with %s not found: %w", podSandboxID, err)
	}

	sb := s.getSandbox(ctx, sandboxID)
	if sb == nil {
		return nil, fmt.Errorf("specified pod sandbox not found: %s", sandboxID)
	}
	if !sb.Created() {
		return nil, errSandboxNotCreated
	}
	return sb, nil
}

// StopMonitors stops all the monitors
func (s *Server) StopMonitors() {
	close(s.monitorsChan)
}

// MonitorsCloseChan returns the close chan for the exit monitor
func (s *Server) MonitorsCloseChan() chan struct{} {
	return s.monitorsChan
}

// StartExitMonitor start a routine that monitors container exits
// and updates the container status
func (s *Server) StartExitMonitor(ctx context.Context) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf(ctx, "Failed to create new watch: %v", err)
	}
	defer watcher.Close()
	done := make(chan struct{})
	go s.monitorExits(ctx, watcher, done)

	if err := watcher.Add(s.config.ContainerExitsDir); err != nil {
		log.Errorf(ctx, "Watcher.Add(%q) failed: %s", s.config.ContainerExitsDir, err)
		close(done)
	}
	<-done
}

func (s *Server) monitorExits(ctx context.Context, watcher *fsnotify.Watcher, done chan struct{}) {
	for {
		select {
		case event := <-watcher.Events:
			go s.handleExit(ctx, event)
		case err := <-watcher.Errors:
			log.Debugf(ctx, "Watch error: %v", err)
			if s.config.EnablePodEvents {
				close(s.ContainerEventsChan)
			}
			close(done)
			return
		case <-s.monitorsChan:
			log.Debugf(ctx, "Closing exit monitor...")
			close(done)
			return
		}
	}
}

func (s *Server) handleExit(ctx context.Context, event fsnotify.Event) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Debugf(ctx, "Event: %v", event)
	if event.Op&fsnotify.Create != fsnotify.Create {
		return
	}
	containerID := filepath.Base(event.Name)
	log.Debugf(ctx, "Container or sandbox exited: %v", containerID)
	c := s.GetContainer(ctx, containerID)
	nriCtr := c
	resource := "container"
	var sb *sandbox.Sandbox
	if c == nil {
		sb = s.GetSandbox(containerID)
		if sb == nil {
			return
		}
		c = sb.InfraContainer()
		resource = "sandbox infra"
	} else {
		sb = s.GetSandbox(c.Sandbox())
	}
	log.Debugf(ctx, "%s exited and found: %v", resource, containerID)

	if err := s.ContainerStateToDisk(ctx, c); err != nil {
		log.Warnf(ctx, "Unable to write %s %s state to disk: %v", resource, c.ID(), err)
	}

	if nriCtr != nil {
		if err := s.nri.stopContainer(ctx, nil, nriCtr); err != nil {
			log.Warnf(ctx, "NRI stop container request of %s failed: %v", nriCtr.ID(), err)
		}
	}

	hooks, err := runtimehandlerhooks.GetRuntimeHandlerHooks(ctx, &s.config, sb.RuntimeHandler(), sb.Annotations())
	if err != nil {
		log.Warnf(ctx, "Failed to get runtime handler %q hooks", sb.RuntimeHandler())
	} else if hooks != nil {
		if err := hooks.PostStop(ctx, c, sb); err != nil {
			log.Errorf(ctx, "Failed to run post-stop hook for container %s: %v", c.ID(), err)
		}
	}

	s.generateCRIEvent(ctx, c, types.ContainerEventType_CONTAINER_STOPPED_EVENT)
	if err := os.Remove(event.Name); err != nil {
		log.Warnf(ctx, "Failed to remove exit file: %v", err)
	}
}

func (s *Server) getSandboxStatuses(ctx context.Context, sandboxID string) (*types.PodSandboxStatus, error) {
	sandboxStatusRequest := &types.PodSandboxStatusRequest{PodSandboxId: sandboxID}
	sandboxStatus, err := s.PodSandboxStatus(ctx, sandboxStatusRequest)

	if isNotFound(err) {
		return nil, err
	}

	if err != nil {
		return nil, fmt.Errorf("getSandboxStatuses: %w", err)
	}

	return sandboxStatus.GetStatus(), nil
}

func (s *Server) getContainerStatuses(ctx context.Context, sandboxUID string) ([]*types.ContainerStatus, error) {
	listContainerRequest := &types.ListContainersRequest{Filter: &types.ContainerFilter{LabelSelector: map[string]string{kubetypes.KubernetesPodUIDLabel: sandboxUID}}}
	containers, err := s.ListContainers(ctx, listContainerRequest)
	if err != nil {
		return []*types.ContainerStatus{}, err
	}

	containerStatuses := []*types.ContainerStatus{}
	for _, cc := range containers.GetContainers() {
		containerStatusRequest := &types.ContainerStatusRequest{ContainerId: cc.Id}
		resp, err := s.ContainerStatus(ctx, containerStatusRequest)
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return []*types.ContainerStatus{}, err
		}
		containerStatuses = append(containerStatuses, resp.GetStatus())
	}

	return containerStatuses, nil
}

func (s *Server) getContainerStatusesFromSandboxID(ctx context.Context, sandboxID string) ([]*types.ContainerStatus, error) {
	listContainerRequest := &types.ListContainersRequest{Filter: &types.ContainerFilter{PodSandboxId: sandboxID}}
	containers, err := s.ListContainers(ctx, listContainerRequest)
	if err != nil {
		return []*types.ContainerStatus{}, err
	}

	containerStatuses := []*types.ContainerStatus{}
	for _, cc := range containers.GetContainers() {
		containerStatusRequest := &types.ContainerStatusRequest{ContainerId: cc.Id, Verbose: false}
		resp, err := s.ContainerStatus(ctx, containerStatusRequest)
		if isNotFound(err) {
			continue
		}
		if err != nil {
			return []*types.ContainerStatus{}, err
		}
		containerStatuses = append(containerStatuses, resp.GetStatus())
	}

	return containerStatuses, nil
}

func (s *Server) generateCRIEvent(ctx context.Context, container *oci.Container, eventType types.ContainerEventType) {
	// returning no error if the Evented PLEG feature is not enabled
	if !s.config.EnablePodEvents {
		return
	}
	if err := s.Runtime().UpdateContainerStatus(ctx, container); err != nil {
		log.Errorf(ctx, "GenerateCRIEvent: event type: %s, failed to update the container status %s: %v", eventType, container.ID(), err)
		return
	}

	if !s.HasSandbox(container.Sandbox()) {
		return
	}

	sandboxStatuses, err := s.getSandboxStatuses(ctx, s.GetSandbox(container.Sandbox()).ID())

	if isNotFound(err) {
		return
	}

	if err != nil {
		log.Errorf(ctx, "GenerateCRIEvent: event type: %s, failed to get sandbox statuses of the pod %s: %v", eventType, sandboxStatuses.Metadata.Uid, err)
		return
	}

	containerStatuses, err := s.getContainerStatuses(ctx, sandboxStatuses.Metadata.Uid)
	if err != nil {
		log.Errorf(ctx, "GenerateCRIEvent: event type: %s, failed to get container statuses of the pod %s: %v", eventType, sandboxStatuses.Metadata.Uid, err)
		return
	}

	select {
	case s.ContainerEventsChan <- types.ContainerEventResponse{ContainerId: container.ID(), ContainerEventType: eventType, CreatedAt: time.Now().UnixNano(), PodSandboxStatus: sandboxStatuses, ContainersStatuses: containerStatuses}:
		log.Debugf(ctx, "Container event %s generated for %s", eventType, container.ID())
	default:
		log.Errorf(ctx, "GenerateCRIEvent: failed to generate event %s for container %s", eventType, container.ID())
		metrics.Instance().MetricContainersEventsDroppedInc()
		return
	}
}

func isNotFound(err error) bool {
	s, ok := status.FromError(err)
	if !ok {
		return ok
	}
	if s.Code() == codes.NotFound {
		return true
	}

	return false
}
