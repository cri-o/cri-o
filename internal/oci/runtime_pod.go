package oci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"

	conmonClient "github.com/containers/conmon-rs/pkg/client"
	conmonconfig "github.com/containers/conmon/runner/config"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"go.podman.io/common/pkg/resize"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	"k8s.io/cri-streaming/pkg/streaming/remotecommand"

	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/opentelemetry"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils"
)

// runtimePod is the Runtime interface implementation relying on conmon-rs to
// interact with the container runtime on a pod level.
type runtimePod struct {
	// embed an oci runtime while runtimePod is built up.
	// This also allows for shared functionality between the two (specific stop behavior)
	// to be easily shared.
	oci       *runtimeOCI
	client    *conmonClient.ConmonClient
	serverDir string
}

// newRuntimePod creates a new runtimePod instance.
func newRuntimePod(r *Runtime, handler *config.RuntimeHandler, c *Container) (RuntimeImpl, error) {
	// If the container is not an infra container, use the client of the infra container.
	if !c.IsInfra() {
		// Search for the runtimePod instance belonging to the sandbox.
		impl, ok := r.runtimeImplMap[c.Sandbox()]
		if !ok {
			// This is a programming error.
			// This code block assumes every container has a conmon client created already.
			panic("Attempted to create a new runtime without creating a pod first")
		}

		return impl, nil
	}

	runRoot := config.DefaultRuntimeRoot
	if handler.RuntimeRoot != "" {
		runRoot = handler.RuntimeRoot
	}

	client, err := conmonClient.New(newConmonServerConfig(r, handler, c, runRoot))
	if err != nil {
		return nil, err
	}

	logrus.Debugf("Running conmonrs with PID: %d", client.PID())

	// TODO FIXME we need to move conmon-rs to the new cgroup
	return &runtimePod{
		oci: &runtimeOCI{
			Runtime: r,
			handler: handler,
			root:    runRoot,
		},
		client:    client,
		serverDir: c.dir,
	}, nil
}

// newConmonServerConfig returns the configuration of the conmon-rs server of
// the pod of the infra container c.
func newConmonServerConfig(
	r *Runtime,
	handler *config.RuntimeHandler,
	c *Container,
	runRoot string,
) *conmonClient.ConmonServerConfig {
	cgroupManager := conmonClient.CgroupManagerSystemd
	if !r.config.CgroupManager().IsSystemd() {
		cgroupManager = conmonClient.CgroupManagerCgroupfs
	}

	heaptrack := &conmonClient.Heaptrack{}
	logDriver := conmonClient.LogDriverNone

	for _, env := range handler.MonitorEnv {
		keyVal := strings.SplitN(env, "=", 2)
		if len(keyVal) != 2 {
			logrus.Warnf("Skipping monitor env %q because it is not in key=value format", env)

			continue
		}

		// Every other entry is only part of the server environment below.
		switch keyVal[0] {
		case monitorEnvLogDriver:
			logDriver = conmonClient.LogDriver(keyVal[1])

		case monitorEnvHeaptrackOutputPath:
			heaptrack.Enabled = true
			heaptrack.OutputPath = filepath.Join(keyVal[1], "cri-o.conmon-rs."+c.ID())

		case monitorEnvHeaptrackBinaryPath:
			heaptrack.Enabled = true
			heaptrack.BinaryPath = keyVal[1]
		}
	}

	return &conmonClient.ConmonServerConfig{
		ConmonServerPath: handler.MonitorPath,
		LogLevel:         conmonClient.FromLogrusLevel(logrus.GetLevel()),
		LogDriver:        logDriver,
		Runtime:          handler.RuntimePath,
		ServerRunDir:     c.dir,
		RuntimeRoot:      runRoot,
		CgroupManager:    cgroupManager,
		Tracing: &conmonClient.Tracing{
			Tracer:   opentelemetry.Tracer(),
			Enabled:  r.config.EnableTracing,
			Endpoint: "http://" + r.config.TracingEndpoint,
		},
		Heaptrack: heaptrack,
		// The server passes its environment on to the OCI runtime and the
		// processes it starts, like conmon does, so it gets the same one
		// instead of the whole environment of CRI-O. The options consumed
		// above only configure the server and are left out.
		Env: slices.DeleteFunc(monitorEnv(handler), isConmonServerOption),
	}
}

// The monitor_env entries which configure the conmon-rs server.
const (
	monitorEnvLogDriver           = "LOG_DRIVER"
	monitorEnvHeaptrackOutputPath = "HEAPTRACK_OUTPUT_PATH"
	monitorEnvHeaptrackBinaryPath = "HEAPTRACK_BINARY_PATH"
)

// isConmonServerOption returns whether the monitor_env entry env configures
// the conmon-rs server rather than being part of its environment.
func isConmonServerOption(env string) bool {
	key, _, found := strings.Cut(env, "=")
	if !found {
		return false
	}

	switch key {
	case monitorEnvLogDriver, monitorEnvHeaptrackOutputPath, monitorEnvHeaptrackBinaryPath:
		return true
	}

	return false
}

// stopAfterStdinEOF returns whether the server closes the container stdin
// once an attach client closed its stdin. It matches conmon, which runs a
// stdin once container without a terminal without --leave-stdin-open. The
// server applies the latest value to every attach of the container, so it
// comes from the container config rather than from the attach request.
func stopAfterStdinEOF(c *Container) bool {
	return c.stdin && c.StdinOnce() && !c.terminal
}

func (r *runtimePod) CreateContainer(
	ctx context.Context,
	c *Container,
	cgroupParent string,
	restore bool,
) error {
	// If this container is the infra container or spoofed,
	// then it is the pod container and we move conmonrs
	// to the right configured cgroup.
	if c.IsInfra() || c.Spoofed() {
		v, err := r.client.Version(ctx, &conmonClient.VersionConfig{Verbose: false})
		if err != nil {
			return fmt.Errorf(
				"failed to get version of client before moving server to cgroup: %w",
				err,
			)
		}

		if v.Tag == "" {
			v.Tag = "none"
		}

		log.Debugf(
			ctx,
			"Using conmonrs version: %s, tag: %s, commit: %s, build: %s, target: %s, %s, %s",
			v.Version, v.Tag, v.Commit, v.BuildDate, v.Target, v.RustVersion, v.CargoVersion,
		)

		// Platform specific container setup
		if err := r.oci.createContainerPlatform(c, cgroupParent, int(v.ProcessID)); err != nil {
			return fmt.Errorf("create container for platform: %w", err)
		}
	}

	if c.Spoofed() {
		return nil
	}

	var maxSize uint64
	if r.oci.config.LogSizeMax >= 0 {
		maxSize = uint64(r.oci.config.LogSizeMax)
	}

	logDrivers := []conmonClient.ContainerLogDriver{
		{
			Type:    conmonClient.LogDriverTypeContainerRuntimeInterface,
			Path:    c.logPath,
			MaxSize: maxSize,
		},
	}
	// Like the journald log path of conmon, with the container name as
	// CONTAINER_NAME, which servers without FeatureJournaldFields ignore.
	if r.oci.config.LogToJournald {
		logDrivers = append(logDrivers, conmonClient.ContainerLogDriver{
			Type:          conmonClient.LogDriverTypeJournald,
			ContainerName: c.name,
		})
	}

	createConfig := &conmonClient.CreateContainerConfig{
		ID:         c.ID(),
		BundlePath: c.bundlePath,
		Terminal:   c.terminal,
		Stdin:      c.stdin,
		ExitPaths: []string{
			filepath.Join(r.oci.config.ContainerExitsDir, c.ID()),
			c.exitFilePath(),
		},
		OOMExitPaths: []string{
			filepath.Join(c.bundlePath, "oom"),
		}, // Keep in sync with location in oci.UpdateContainerStatus()
		LogDrivers: logDrivers,
	}
	resp, err := r.client.CreateContainer(ctx, createConfig)
	// TODO FIXME do we need to cleanup the container?
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	// Now we know the container has started, save the pid to verify against future calls.
	if err := c.state.SetInitPid(int(resp.PID)); err != nil {
		return fmt.Errorf("set init PID: %w", err)
	}

	c.state.ContainerMonitorProcess, err = r.getConmonrsProcess()
	if err != nil {
		return err
	}

	c.SetMonitorProcess(ctx)

	return nil
}

func (r *runtimePod) getConmonrsProcess() (*ContainerMonitorProcess, error) {
	conmonrsPid := int(r.client.PID())

	startTime, err := getPidStartTime(conmonrsPid)
	if err != nil {
		return nil, fmt.Errorf("failed to get conmonrs pid start time: %w", err)
	}

	return &ContainerMonitorProcess{
		Pid:       conmonrsPid,
		StartTime: startTime,
	}, nil
}

func (r *runtimePod) StartContainer(ctx context.Context, c *Container) error {
	return r.oci.StartContainer(ctx, c)
}

func (r *runtimePod) CheckpointContainer(
	ctx context.Context,
	c *Container,
	specgen *rspec.Spec,
	leaveRunning bool,
) error {
	return r.oci.CheckpointContainer(ctx, c, specgen, leaveRunning)
}

func (r *runtimePod) RestoreContainer(
	ctx context.Context,
	c *Container,
	cgroupParent string,
	mountLabel string,
) error {
	return r.oci.RestoreContainer(ctx, c, cgroupParent, mountLabel)
}

func (r *runtimePod) ExecContainer(
	ctx context.Context,
	c *Container,
	cmd []string,
	stdin io.Reader,
	stdout, stderr io.WriteCloser,
	tty bool,
	resizeChan <-chan remotecommand.TerminalSize,
) error {
	return r.oci.ExecContainer(ctx, c, cmd, stdin, stdout, stderr, tty, resizeChan)
}

func (r *runtimePod) ExecSyncContainer(
	ctx context.Context,
	c *Container,
	cmd []string,
	timeout int64,
) (*types.ExecSyncResponse, error) {
	if c.Spoofed() {
		return nil, nil
	}
	// TODO FIXME
	if timeout < 0 {
		return nil, errors.New("timeout cannot be negative")
	}

	res, err := r.client.ExecSyncContainer(ctx, &conmonClient.ExecSyncConfig{
		ID:       c.ID(),
		Command:  cmd,
		Timeout:  uint64(timeout),
		Terminal: c.terminal,
	})
	if err != nil {
		return nil, err
	}

	return execSyncResponse(res)
}

// execSyncResponse turns the result of an exec sync into the response of the
// kubelet like for conmon: a timeout is a failed command, which the probes
// treat as unhealthy, while a command which did not start or whose exit the
// server could not tell is an error.
func execSyncResponse(res *conmonClient.ExecContainerResult) (*types.ExecSyncResponse, error) {
	if res.TimedOut {
		return &types.ExecSyncResponse{
			Stderr:   []byte(conmonconfig.TimedOutMessage),
			ExitCode: -1,
		}, nil
	}

	var execErr error

	switch res.ExitCode {
	case conmonClient.ExecExitCodeNotStarted:
		execErr = errors.New("exec did not start")
	case conmonClient.ExecExitCodeTimedOut, -1:
		execErr = errors.New("exit status of exec unknown")
	}

	if execErr != nil {
		syncErr := &ExecSyncError{ExitCode: -1, Err: execErr}
		syncErr.Stdout.Write(res.Stdout)
		syncErr.Stderr.Write(res.Stderr)

		return nil, syncErr
	}

	return &types.ExecSyncResponse{
		ExitCode: res.ExitCode,
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
	}, nil
}

func (r *runtimePod) UpdateContainer(
	ctx context.Context,
	c *Container,
	res *rspec.LinuxResources,
) error {
	return r.oci.UpdateContainer(ctx, c, res)
}

func (r *runtimePod) StopContainer(ctx context.Context, c *Container, timeout int64) error {
	return r.oci.StopContainer(ctx, c, timeout)
}

func (r *runtimePod) DeleteContainer(ctx context.Context, c *Container) error {
	if err := r.oci.DeleteContainer(ctx, c); err != nil {
		return fmt.Errorf("delete container: %w", err)
	}
	// Shutdown the runtime if the infra container is being deleted
	if c.IsInfra() {
		return shutdownConmonServer(ctx, r.client)
	}

	return nil
}

// conmonServerStopper stops the conmon-rs server of a pod.
type conmonServerStopper interface {
	PID() uint32
	ShutdownContext(ctx context.Context) error
	KillContext(ctx context.Context) error
	Close() error
}

// conmonServerKillTimeout bounds the wait for a killed conmon-rs server to
// exit. SIGKILL cannot be caught, so a server which is still around after it
// is stuck in the kernel and waiting longer does not help.
const conmonServerKillTimeout = 3 * time.Second

// shutdownConmonServer stops the conmon-rs server of the pod and closes the
// client once the server is gone. A server which does not exit in time gets
// killed, otherwise it would linger. If stopping the server fails, the client
// stays open and keeps its pidfd of the server: the runtime implementation
// of the pod stays in place as well, so that the retry of the pod removal can
// stop the server again.
func shutdownConmonServer(ctx context.Context, client conmonServerStopper) error {
	// Stopping the server has to finish even if the request which deletes
	// the container got canceled, otherwise the shutdown fails with the
	// error of the context instead of timing out and the server keeps
	// running.
	ctx = context.WithoutCancel(ctx)

	if err := stopConmonServer(ctx, client); err != nil {
		return err
	}

	if err := client.Close(); err != nil {
		log.Warnf(ctx, "Unable to close conmon-rs client: %v", err)
	}

	return nil
}

// stopConmonServer stops the conmon-rs server of the pod and kills it if it
// does not exit in time.
func stopConmonServer(ctx context.Context, client conmonServerStopper) error {
	err := client.ShutdownContext(ctx)
	if err == nil {
		return nil
	}

	// Without its PID the server cannot be signaled, and a retry of the
	// removal would fail the same way.
	if errors.Is(err, conmonClient.ErrServerPIDUnknown) {
		log.Warnf(ctx, "Unable to stop conmon-rs server: %v", err)

		return nil
	}

	if !errors.Is(err, conmonClient.ErrShutdownTimeout) {
		return fmt.Errorf("failed to shutdown client: %w", err)
	}

	log.Warnf(
		ctx,
		"Killing conmon-rs server with PID %d after shutdown failed: %v",
		client.PID(),
		err,
	)

	killCtx, cancel := context.WithTimeout(ctx, conmonServerKillTimeout)
	defer cancel()

	if err := client.KillContext(killCtx); err != nil {
		return fmt.Errorf("failed to kill conmon-rs server: %w", err)
	}

	return nil
}

func (r *runtimePod) UpdateContainerStatus(ctx context.Context, c *Container) error {
	return r.oci.UpdateContainerStatus(ctx, c)
}

func (r *runtimePod) PauseContainer(ctx context.Context, c *Container) error {
	return r.oci.PauseContainer(ctx, c)
}

func (r *runtimePod) UnpauseContainer(ctx context.Context, c *Container) error {
	return r.oci.UnpauseContainer(ctx, c)
}

func (r *runtimePod) CgroupStats(
	ctx context.Context,
	c *Container,
	cgroup string,
) (*stats.CgroupStats, error) {
	return r.oci.CgroupStats(ctx, c, cgroup)
}

func (r *runtimePod) DiskStats(
	ctx context.Context,
	c *Container,
	cgroup string,
) (*stats.DiskStats, error) {
	return r.oci.DiskStats(ctx, c, cgroup)
}

func (r *runtimePod) AttachContainer(
	ctx context.Context,
	c *Container,
	inputStream io.Reader,
	outputStream, errorStream io.WriteCloser,
	tty bool,
	resizeChan <-chan remotecommand.TerminalSize,
) error {
	attachSocketPath := filepath.Join(r.serverDir, c.ID(), "attach")
	libpodResize := make(chan resize.TerminalSize, 1)

	// The client stops reading the resize events once the attach returned,
	// so that a later event must not block the goroutine of HandleResizing.
	done := make(chan struct{})
	defer close(done)

	utils.HandleResizing(resizeChan, func(size remotecommand.TerminalSize) {
		select {
		case libpodResize <- resize.TerminalSize{Width: size.Width, Height: size.Height}:
		case <-done:
		}
	})

	var (
		stdin          *conmonClient.In
		stdout, stderr *conmonClient.Out
	)

	if inputStream != nil {
		stdin = &conmonClient.In{ReadCloser: io.NopCloser(inputStream)}
	}

	if outputStream != nil {
		stdout = &conmonClient.Out{WriteCloser: outputStream}
	}

	if errorStream != nil {
		stderr = &conmonClient.Out{WriteCloser: errorStream}
	}

	return r.client.AttachContainer(ctx, &conmonClient.AttachConfig{
		ID:                c.ID(),
		SocketPath:        attachSocketPath,
		Tty:               tty,
		StopAfterStdinEOF: stopAfterStdinEOF(c),
		ContainerStdin:    c.stdin,
		Resize:            libpodResize,
		Streams: conmonClient.AttachStreams{
			Stdin:  stdin,
			Stdout: stdout,
			Stderr: stderr,
		},
	})
}

func (r *runtimePod) PortForwardContainer(
	ctx context.Context,
	c *Container,
	netNsPath string,
	port int32,
	stream io.ReadWriteCloser,
) error {
	return r.oci.PortForwardContainer(ctx, c, netNsPath, port, stream)
}

func (r *runtimePod) ReopenContainerLog(ctx context.Context, c *Container) error {
	return r.client.ReopenLogContainer(ctx, &conmonClient.ReopenLogContainerConfig{
		ID: c.ID(),
	})
}

func (r *runtimePod) IsContainerAlive(c *Container) bool {
	return c.Living() == nil
}

func (r *runtimePod) ProbeMonitor(ctx context.Context, c *Container) error {
	return r.oci.ProbeMonitor(ctx, c)
}

func (r *runtimePod) ServeExecContainer(
	ctx context.Context,
	c *Container,
	cmd []string,
	tty, stdin, stdout, stderr bool,
) (string, error) {
	if err := requireStreamingProtocols(r.client); err != nil {
		return "", err
	}

	res, err := r.client.ServeExecContainer(ctx, &conmonClient.ServeExecContainerConfig{
		ID:      c.ID(),
		Command: cmd,
		Tty:     tty,
		Stdin:   stdin,
		Stdout:  stdout,
		Stderr:  stderr,
		// Older servers ignore it.
		IdleTimeout: r.oci.config.GetStreamIdleTimeout(),
	})
	if err != nil {
		return "", unsupportedError(fmt.Errorf("call ServeExecContainer RPC: %w", err))
	}

	return res.URL, nil
}

// conmonFeatureChecker reports the features of the conmon-rs server of a pod.
type conmonFeatureChecker interface {
	SupportsFeature(feature string) bool
}

// requireStreamingProtocols fails with errors.ErrUnsupported if the running
// conmon-rs server of the pod does not speak the streaming protocols of the
// kubelet. The server keeps running across upgrades of conmon-rs, so it can
// be older than the binary on disk: before v1.2.0, it only spoke the v5
// WebSocket protocol for exec and attach.
func requireStreamingProtocols(client conmonFeatureChecker) error {
	if !client.SupportsFeature(conmonClient.FeatureStreamingProtocols) {
		return unsupportedError(fmt.Errorf(
			"conmon-rs server does not report the %s feature: %w",
			conmonClient.FeatureStreamingProtocols,
			conmonClient.ErrUnsupported,
		))
	}

	return nil
}

// unsupportedError adds errors.ErrUnsupported to an error of the conmon-rs
// client which tells that the server does not support a feature, so that the
// callers detect it like for the other runtime types.
func unsupportedError(err error) error {
	if errors.Is(err, conmonClient.ErrUnsupported) {
		return fmt.Errorf("%w: %w", err, errors.ErrUnsupported)
	}

	return err
}

func (r *runtimePod) ServeAttachContainer(
	ctx context.Context,
	c *Container,
	stdin, stdout, stderr bool,
) (string, error) {
	if err := requireStreamingProtocols(r.client); err != nil {
		return "", err
	}

	res, err := r.client.ServeAttachContainer(ctx, &conmonClient.ServeAttachContainerConfig{
		ID:                c.ID(),
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		StopAfterStdinEOF: stopAfterStdinEOF(c),
		// Older servers ignore it.
		IdleTimeout: r.oci.config.GetStreamIdleTimeout(),
	})
	if err != nil {
		return "", unsupportedError(fmt.Errorf("call ServeAttachContainer RPC: %w", err))
	}

	return res.URL, nil
}

func (r *runtimePod) ServePortForwardContainer(
	ctx context.Context,
	c *Container,
	netNsPath string,
) (string, error) {
	res, err := r.client.ServePortForwardContainer(
		ctx,
		&conmonClient.ServePortForwardContainerConfig{
			NetNsPath:   netNsPath,
			IdleTimeout: r.oci.config.GetStreamIdleTimeout(),
		},
	)
	if err != nil {
		return "", unsupportedError(fmt.Errorf("call ServePortForwardContainer RPC: %w", err))
	}

	return res.URL, nil
}
