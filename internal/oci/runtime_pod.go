package oci

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"syscall"

	"github.com/containers/common/pkg/resize"
	conmonClient "github.com/containers/conmon-rs/pkg/client"
	conmonconfig "github.com/containers/conmon/runner/config"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
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

	cgroupManager := conmonClient.CgroupManagerSystemd
	if !r.config.CgroupManager().IsSystemd() {
		cgroupManager = conmonClient.CgroupManagerCgroupfs
	}

	client, err := conmonClient.New(&conmonClient.ConmonServerConfig{
		ConmonServerPath: handler.MonitorPath,
		LogLevel:         conmonClient.FromLogrusLevel(logrus.GetLevel()),
		LogDriver:        conmonClient.LogDriverSystemd,
		Runtime:          handler.RuntimePath,
		ServerRunDir:     c.dir,
		RuntimeRoot:      runRoot,
		CgroupManager:    cgroupManager,
		Tracing: &conmonClient.Tracing{
			Tracer:   opentelemetry.Tracer(),
			Enabled:  r.config.EnableTracing,
			Endpoint: "http://" + r.config.TracingEndpoint,
		},
	})
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

func (r *runtimePod) CreateContainer(ctx context.Context, c *Container, cgroupParent string, restore bool) error {
	// If this container is the infra container or spoofed,
	// then it is the pod container and we move conmonrs
	// to the right configured cgroup.
	if c.IsInfra() || c.Spoofed() {
		v, err := r.client.Version(ctx, &conmonClient.VersionConfig{Verbose: false})
		if err != nil {
			return fmt.Errorf("failed to get version of client before moving server to cgroup: %w", err)
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

	createConfig := &conmonClient.CreateContainerConfig{
		ID:           c.ID(),
		BundlePath:   c.bundlePath,
		Terminal:     c.terminal,
		Stdin:        c.stdin,
		ExitPaths:    []string{filepath.Join(r.oci.config.ContainerExitsDir, c.ID()), c.exitFilePath()},
		OOMExitPaths: []string{filepath.Join(c.bundlePath, "oom")}, // Keep in sync with location in oci.UpdateContainerStatus()
		LogDrivers: []conmonClient.ContainerLogDriver{
			{
				Type:    conmonClient.LogDriverTypeContainerRuntimeInterface,
				Path:    c.logPath,
				MaxSize: maxSize,
			},
		},
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

	return nil
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

func (r *runtimePod) ExecContainer(ctx context.Context, c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error {
	return r.oci.ExecContainer(ctx, c, cmd, stdin, stdout, stderr, tty, resizeChan)
}

func (r *runtimePod) ExecSyncContainer(ctx context.Context, c *Container, cmd []string, timeout int64) (*types.ExecSyncResponse, error) {
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

	if res.TimedOut {
		return &types.ExecSyncResponse{
			Stderr:   []byte(conmonconfig.TimedOutMessage),
			ExitCode: -1,
		}, nil
	}

	return &types.ExecSyncResponse{
		ExitCode: res.ExitCode,
		Stdout:   res.Stdout,
		Stderr:   res.Stderr,
	}, nil
}

func (r *runtimePod) UpdateContainer(ctx context.Context, c *Container, res *rspec.LinuxResources) error {
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
		if err := r.client.Shutdown(); err != nil {
			return fmt.Errorf("failed to shutdown client: %w", err)
		}
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

func (r *runtimePod) ContainerStats(ctx context.Context, c *Container, cgroup string) (*cgmgr.CgroupStats, error) {
	return r.oci.ContainerStats(ctx, c, cgroup)
}

func (r *runtimePod) SignalContainer(ctx context.Context, c *Container, sig syscall.Signal) error {
	return r.oci.SignalContainer(ctx, c, sig)
}

func (r *runtimePod) AttachContainer(ctx context.Context, c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resizeChan <-chan remotecommand.TerminalSize) error {
	attachSocketPath := filepath.Join(r.serverDir, c.ID(), "attach")
	libpodResize := make(chan resize.TerminalSize, 1)

	utils.HandleResizing(resizeChan, func(size remotecommand.TerminalSize) {
		var libpodEvent resize.TerminalSize
		libpodEvent.Height = size.Height
		libpodEvent.Width = size.Width
		libpodResize <- libpodEvent
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
		StopAfterStdinEOF: c.stdin && c.StdinOnce() && !tty,
		ContainerStdin:    c.stdin,
		Resize:            libpodResize,
		Streams: conmonClient.AttachStreams{
			Stdin:  stdin,
			Stdout: stdout,
			Stderr: stderr,
		},
	})
}

func (r *runtimePod) PortForwardContainer(ctx context.Context, c *Container, netNsPath string, port int32, stream io.ReadWriteCloser) error {
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
	// Not implemented
	return nil
}
