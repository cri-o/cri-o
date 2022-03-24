package oci

import (
	"fmt"
	"io"
	"path/filepath"
	"syscall"

	conmonClient "github.com/containers/conmon-rs/pkg/client"
	conmonconfig "github.com/containers/conmon/runner/config"
	"github.com/containers/podman/v3/libpod/define"
	"github.com/cri-o/cri-o/pkg/config"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"k8s.io/client-go/tools/remotecommand"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
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

// newRuntimePod creates a new runtimePod instance
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

	client, err := conmonClient.New(&conmonClient.ConmonServerConfig{
		ConmonServerPath: handler.MonitorPath,
		LogLevel:         logrus.GetLevel().String(),
		Runtime:          handler.RuntimePath,
		ServerRunDir:     c.dir,
		RuntimeRoot:      runRoot,
	})
	if err != nil {
		return nil, err
	}

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

func (r *runtimePod) CreateContainer(ctx context.Context, c *Container, cgroupParent string) error {
	// If this container is the infra container, all that needs to be done is move conmonrs to the pod cgroup
	if c.IsInfra() {
		v, err := r.client.Version(ctx)
		if err != nil {
			return fmt.Errorf("failed to get version of client before moving server to cgroup: %v", err)
		}
		// Platform specific container setup
		if err := r.oci.createContainerPlatform(c, cgroupParent, int(v.ProcessID)); err != nil {
			return err
		}
	}
	if c.Spoofed() {
		return nil
	}
	createConfig := &conmonClient.CreateContainerConfig{
		ID:           c.ID(),
		BundlePath:   c.bundlePath,
		Terminal:     c.terminal,
		ExitPaths:    []string{filepath.Join(r.oci.config.ContainerExitsDir, c.ID()), c.exitFilePath()},
		OOMExitPaths: []string{filepath.Join(c.bundlePath, "oom")}, // Keep in sync with location in oci.UpdateContainerStatus()
		LogDrivers: []conmonClient.LogDriver{
			{
				Type: conmonClient.LogDriverTypeContainerRuntimeInterface,
				Path: c.logPath,
			},
		},
	}
	resp, err := r.client.CreateContainer(ctx, createConfig)
	// TODO FIXME do we need to cleanup the container?
	if err != nil {
		return err
	}
	// Now we know the container has started, save the pid to verify against future calls.
	if err := c.state.SetInitPid(int(resp.PID)); err != nil {
		return err
	}
	return nil
}

func (r *runtimePod) StartContainer(ctx context.Context, c *Container) error {
	return r.oci.StartContainer(ctx, c)
}

func (r *runtimePod) ExecContainer(ctx context.Context, c *Container, cmd []string, stdin io.Reader, stdout, stderr io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	return r.oci.ExecContainer(ctx, c, cmd, stdin, stdout, stderr, tty, resize)
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
		return err
	}
	// Shutdown the runtime if the infra container is being deleted
	if c.IsInfra() {
		if err := r.client.Shutdown(); err != nil {
			return fmt.Errorf("failed to shutdown client: %v", err)
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

func (r *runtimePod) ContainerStats(ctx context.Context, c *Container, cgroup string) (*types.ContainerStats, error) {
	return r.oci.ContainerStats(ctx, c, cgroup)
}

func (r *runtimePod) SignalContainer(ctx context.Context, c *Container, sig syscall.Signal) error {
	return r.oci.SignalContainer(ctx, c, sig)
}

func (r *runtimePod) AttachContainer(ctx context.Context, c *Container, inputStream io.Reader, outputStream, errorStream io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize) error {
	attachSocketPath := filepath.Join(r.serverDir, c.ID(), "attach")
	libpodResize := make(chan define.TerminalSize, 1)
	go func() {
		var event remotecommand.TerminalSize
		var libpodEvent define.TerminalSize

		for event = range resize {
			libpodEvent.Height = event.Height
			libpodEvent.Width = event.Width
			libpodResize <- libpodEvent
		}
	}()

	return r.client.AttachContainer(ctx, &conmonClient.AttachConfig{
		ID:                c.ID(),
		SocketPath:        attachSocketPath,
		Tty:               tty,
		StopAfterStdinEOF: c.stdin && !c.StdinOnce() && !tty,
		Resize:            libpodResize,
		Streams: conmonClient.AttachStreams{
			Stdin:  &conmonClient.In{Reader: inputStream},
			Stdout: &conmonClient.Out{WriteCloser: outputStream},
			Stderr: &conmonClient.Out{WriteCloser: errorStream},
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
