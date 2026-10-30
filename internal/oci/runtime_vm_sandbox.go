package oci

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	sandbox "github.com/containerd/containerd/api/runtime/sandbox/v1"
	"github.com/containerd/containerd/api/runtime/task/v2"
	client "github.com/containerd/containerd/runtime/v2/shim"
	"github.com/containerd/ttrpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// SandboxConfigFile preserves the CRI configuration for the shim Sandbox API.
const SandboxConfigFile = "cri-sandbox-config.pb"

// ConfigureSandboxRuntime associates a dropped infra container with its VM shim.
func (c *Container) ConfigureSandboxRuntime(handler, bundle string) {
	c.runtimeHandler = handler
	c.bundlePath = bundle
}

// usesSandboxAPI follows the persisted sandbox mode across handler changes.
func usesSandboxAPI(c *Container) bool {
	return c.Spoofed() && c.BundlePath() != ""
}

func sandboxGone(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED)
}

// closeSandboxClient discards the service as well so the next call reconnects.
// The caller must hold the sandbox container's opLock.
func (r *runtimeVM) closeSandboxClient() error {
	err := r.client.Close()
	r.sandbox = nil
	r.client = nil
	r.task = nil

	return err
}

func (r *runtimeVM) connectSandbox(c *Container) error {
	if r.sandbox != nil {
		return nil
	}

	data, err := os.ReadFile(filepath.Join(c.BundlePath(), "address"))
	if err != nil {
		return fmt.Errorf("read sandbox shim address: %w", err)
	}

	conn, err := client.Connect(strings.TrimSpace(string(data)), client.AnonDialer)
	if err != nil {
		return fmt.Errorf("connect sandbox shim: %w", err)
	}

	r.client = ttrpc.NewClient(conn, ttrpc.WithOnClose(func() { conn.Close() }))
	r.task = task.NewTaskClient(r.client)
	r.sandbox = sandbox.NewTTRPCSandboxClient(r.client)

	return nil
}

func sandboxRequest(c *Container, cgroupParent string) (*sandbox.CreateSandboxRequest, error) {
	data, err := os.ReadFile(filepath.Join(c.BundlePath(), SandboxConfigFile))
	if err != nil {
		return nil, fmt.Errorf("read CRI sandbox configuration: %w", err)
	}

	config := &types.PodSandboxConfig{}
	if err := proto.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("decode CRI sandbox configuration: %w", err)
	}

	if config.GetLinux() == nil {
		config.Linux = &types.LinuxPodSandboxConfig{}
	}

	config.Linux.CgroupParent = cgroupParent
	if spec := c.Spec(); spec.Linux != nil && spec.Linux.CgroupsPath != "" {
		config.Linux.CgroupParent = spec.Linux.CgroupsPath
	}

	data, err = proto.Marshal(config)
	if err != nil {
		return nil, err
	}

	spec := c.Spec()
	netns := ""

	if spec.Linux != nil {
		for _, ns := range spec.Linux.Namespaces {
			if ns.Type == "network" {
				netns = ns.Path

				break
			}
		}
	}

	if netns == "" {
		return nil, errors.New("Sandbox API requires a pod network namespace")
	}

	return &sandbox.CreateSandboxRequest{
		SandboxID: c.ID(), BundlePath: c.BundlePath(), NetnsPath: netns,
		Options:     &anypb.Any{TypeUrl: "runtime.v1.PodSandboxConfig", Value: data},
		Annotations: config.GetAnnotations(),
	}, nil
}

func (r *runtimeVM) createSandbox(
	ctx context.Context,
	c *Container,
	cgroupParent string,
) (retErr error) {
	request, err := sandboxRequest(c, cgroupParent)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(
		ctx,
		time.Duration(r.handler.ContainerCreateTimeout)*time.Second,
	)
	defer cancel()

	if err := r.startRuntimeDaemon(ctx, c); err != nil {
		return fmt.Errorf("start sandbox shim: %w", err)
	}

	r.sandbox = sandbox.NewTTRPCSandboxClient(r.client)

	defer func() {
		if retErr != nil {
			r.cleanupFailedSandbox(ctx, c)
		}
	}()

	if _, err := r.sandbox.CreateSandbox(ctx, request); err != nil {
		return fmt.Errorf("create sandbox: %w", err)
	}

	c.state.Status = ContainerStateCreated

	return nil
}

func (r *runtimeVM) startSandbox(ctx context.Context, c *Container) (retErr error) {
	if err := r.connectSandbox(c); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(
		ctx,
		time.Duration(r.handler.ContainerCreateTimeout)*time.Second,
	)
	defer cancel()

	response, err := r.sandbox.StartSandbox(ctx, &sandbox.StartSandboxRequest{SandboxID: c.ID()})
	if err != nil {
		r.cleanupFailedSandbox(ctx, c)

		return fmt.Errorf("start sandbox: %w", err)
	}

	if err := c.state.SetInitPid(int(response.GetPid())); err != nil {
		r.cleanupFailedSandbox(ctx, c)

		return err
	}

	c.state.Status = ContainerStateRunning
	c.state.Started = response.GetCreatedAt().AsTime()
	r.watchSandbox(c)

	return nil
}

func (r *runtimeVM) stopSandbox(ctx context.Context, c *Container, timeout int64) error {
	if c.state.Status == ContainerStateStopped {
		return nil
	}

	if err := r.connectSandbox(c); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second+10*time.Second)
	defer cancel()

	if _, err := r.sandbox.StopSandbox(
		ctx,
		&sandbox.StopSandboxRequest{SandboxID: c.ID(), TimeoutSecs: uint32(max(timeout, 0))},
	); err != nil {
		return fmt.Errorf("stop sandbox: %w", err)
	}

	c.state.Status = ContainerStateStopped
	c.state.Finished = time.Now()

	return nil
}

func (r *runtimeVM) deleteSandbox(ctx context.Context, c *Container) error {
	if err := r.connectSandbox(c); err != nil {
		if sandboxGone(err) {
			return nil
		}

		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if _, err := r.sandbox.ShutdownSandbox(
		ctx,
		&sandbox.ShutdownSandboxRequest{SandboxID: c.ID()},
	); err != nil &&
		!errors.Is(err, ttrpc.ErrClosed) {
		return fmt.Errorf("shutdown sandbox: %w", err)
	}

	return r.closeSandboxClient()
}

func (r *runtimeVM) sandboxStatus(ctx context.Context, c *Container) error {
	if err := r.connectSandbox(c); err != nil {
		if sandboxGone(err) {
			markContainerStopped(c)
			r.writeSandboxExit(c)

			return nil
		}

		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	response, err := r.sandbox.SandboxStatus(ctx, &sandbox.SandboxStatusRequest{SandboxID: c.ID()})
	if err != nil {
		return fmt.Errorf("sandbox status: %w", err)
	}

	c.state.Pid = int(response.GetPid())
	switch strings.ToUpper(response.GetState()) {
	case "SANDBOX_READY", "RUNNING":
		c.state.Status = ContainerStateRunning
		r.watchSandbox(c)
	case "SANDBOX_NOTREADY", "STOPPED":
		markContainerStopped(c)

		if exited := response.GetExitedAt(); exited != nil {
			c.state.Finished = exited.AsTime()
		}
	default:
		return fmt.Errorf("unrecognized sandbox state %q", response.GetState())
	}

	return nil
}

// A shim may not have a runtime instance after a failed CreateSandbox.
// Its task Shutdown API must still release the shim in that case.
func (r *runtimeVM) cleanupFailedSandbox(ctx context.Context, c *Container) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	if _, err := r.sandbox.StopSandbox(
		cleanupCtx,
		&sandbox.StopSandboxRequest{SandboxID: c.ID()},
	); err != nil {
		log.Warnf(ctx, "Stop failed sandbox %s: %v", c.ID(), err)
	}

	if _, err := r.sandbox.ShutdownSandbox(
		cleanupCtx,
		&sandbox.ShutdownSandboxRequest{SandboxID: c.ID()},
	); err != nil {
		if _, shutdownErr := r.task.Shutdown(
			cleanupCtx,
			&task.ShutdownRequest{ID: c.ID(), Now: true},
		); shutdownErr != nil {
			log.Warnf(
				ctx,
				"Shutdown failed sandbox %s: sandbox=%v task=%v",
				c.ID(),
				err,
				shutdownErr,
			)
		}
	}

	if err := r.closeSandboxClient(); err != nil {
		log.Warnf(ctx, "Close failed sandbox %s: %v", c.ID(), err)
	}
}

// watchSandbox starts one waiter at a time. All callers hold c.opLock.
func (r *runtimeVM) watchSandbox(c *Container) {
	if r.sandboxWatch {
		return
	}

	r.sandboxWatch = true
	go r.waitSandbox(c)
}

// waitSandbox reconnects after a broken wait without declaring a live pod dead.
func (r *runtimeVM) waitSandbox(c *Container) {
	defer func() {
		c.opLock.Lock()
		r.sandboxWatch = false
		c.opLock.Unlock()
	}()

	for {
		c.opLock.Lock()
		err := r.connectSandbox(c)
		service := r.sandbox
		c.opLock.Unlock()

		var result *sandbox.WaitSandboxResponse
		if err == nil {
			result, err = service.WaitSandbox(r.ctx, &sandbox.WaitSandboxRequest{SandboxID: c.ID()})
		}

		c.opLock.Lock()
		if err == nil {
			c.state.Status = ContainerStateStopped
			c.state.Finished = result.GetExitedAt().AsTime()
			exitCode := int32(result.GetExitStatus())
			c.state.ExitCode = &exitCode
		} else if r.ctx.Err() == nil {
			log.Warnf(r.ctx, "Wait sandbox %s: %v", c.ID(), err)

			if errors.Is(err, ttrpc.ErrClosed) && r.sandbox == service && r.client != nil {
				if closeErr := r.closeSandboxClient(); closeErr != nil {
					log.Warnf(r.ctx, "Close sandbox %s client: %v", c.ID(), closeErr)
				}
			}

			if c.state.Status != ContainerStateStopped {
				if statusErr := r.sandboxStatus(r.ctx, c); statusErr != nil {
					log.Warnf(r.ctx, "Check sandbox %s after wait failure: %v", c.ID(), statusErr)
				}
			}

			if c.state.Status == ContainerStateStopped {
				markContainerStopped(c)
			}
		}

		stopped := c.state.Status == ContainerStateStopped
		c.opLock.Unlock()

		if stopped {
			r.writeSandboxExit(c)

			return
		}

		select {
		case <-r.ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (r *runtimeVM) writeSandboxExit(c *Container) {
	if err := os.WriteFile(
		filepath.Join(r.exitsPath, c.ID()),
		[]byte("Exited"),
		0o644,
	); err != nil {
		log.Warnf(r.ctx, "Write sandbox %s exit file: %v", c.ID(), err)
	}
}
