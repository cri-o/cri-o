package client

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/containers/conmon-rs/internal/proto"
)

// CreateContainerConfig is the configuration for calling the CreateContainer
// method.
type CreateContainerConfig struct {
	// ID is the container identifier, which the runtime gets as an
	// argument. Servers since v1.2.1 reject an empty one and one which
	// starts with "-", which the runtime would take for an option, servers
	// since v1.2.3 also one with a control character, like a line
	// break.
	ID string

	// BundlePath is the absolute path to the filesystem bundle. Servers
	// since v1.2.1 reject other ones.
	BundlePath string

	// Terminal indicates if a tty should be used or not.
	Terminal bool

	// Stdin indicates if stdin should be available or not.
	Stdin bool

	// ExitPaths is a slice of paths to write the exit statuses. Each file
	// gets the exit code of the container as decimal text once it exited,
	// 128 plus the number of the signal which killed it, like a shell
	// reports it, also for real-time signals since v1.2.2, which gave
	// -1 before. Since v1.2.3, also a traced process, like one under
	// strace, gets its exit code instead of -1. The paths have to be
	// absolute, servers since v1.2.1 reject other ones. Servers since
	// v1.2.3 also reject a path here or in OOMExitPaths which does not
	// name a file, like "/" or one which ends in "..", "/" or "/.".
	ExitPaths []string

	// OOMExitPaths is a slice of files that should be created if the given
	// container is OOM killed. They are created empty before the exit files
	// get written. The paths have to be absolute, servers since v1.2.1
	// reject other ones. Servers since v1.2.3 only detect an OOM kill
	// with cgroup v2 at /sys/fs/cgroup.
	OOMExitPaths []string

	// LogDrivers is a slice of selected log drivers.
	LogDrivers []ContainerLogDriver

	// CommandArgs are the values of the --runtime-opt flag of conmon, like
	// --no-pivot or the SELinux options of runc: the server appends them to
	// the create command of the runtime after its own options, or to the
	// restore command with a RestorePath.
	CommandArgs []string

	// RestorePath is the absolute path of the checkpoint to restore the
	// container from, like the --restore of conmon. With one, the server
	// runs the restore command of the runtime instead of create, detached,
	// with the checkpoint as its image path and the bundle as its work path,
	// and monitors the restored container like a created one. The container
	// runs once CreateContainer returns, the caller does not start it. A
	// server without FeatureRestore fails the call with an error matching
	// ErrUnsupported, it would create a new container instead. Since
	// v1.2.3. Empty for a create.
	RestorePath string
}

// ContainerLogDriver specifies a selected logging mechanism.
type ContainerLogDriver struct {
	// Type defines the log driver variant.
	Type LogDriverType

	// Path specifies the absolute filesystem path of the log driver.
	// Servers since v1.2.1 reject other ones.
	Path string

	// MaxSize is the maximum amount of bytes of the log file. Once the next
	// entry would exceed it, the server replaces the file with a new empty
	// one and keeps no copy of the old one. 0 translates to an unlimited
	// size.
	MaxSize uint64

	// ContainerName is written as CONTAINER_NAME by LogDriverTypeJournald,
	// like the name of conmon. Servers since v1.2.2 also use it as the
	// SYSLOG_IDENTIFIER, and the short container ID without one, older ones
	// "conmonrs". Servers before v1.2.1 ignore it.
	ContainerName string
}

// LogDriverType specifies available log drivers.
type LogDriverType int

const (
	// LogDriverTypeContainerRuntimeInterface is the Kubernetes CRI logger
	// type.
	LogDriverTypeContainerRuntimeInterface LogDriverType = 0

	// LogDriverTypeJournald writes to the systemd journal, the path of the
	// driver is not used. The value 1 belonged to the retired JSON logger.
	LogDriverTypeJournald LogDriverType = 2
)

// CreateContainerResult is the result for calling the CreateContainer method.
type CreateContainerResult struct {
	// PID is the container process identifier: of the process which the
	// runtime created but did not start yet, or which it restored from the
	// RestorePath and which runs already.
	PID uint32
}

// CreateContainer creates a container by running the create command of the
// runtime and monitors it from then on. The container process waits for the
// start command of the runtime, which the caller runs, before it runs the
// command of the container. The server writes the exit and OOM files once the
// container exited.
//
// With a RestorePath, it restores the container from the checkpoint with the
// restore command of the runtime instead, the container runs once the call
// returns. This needs FeatureRestore, see CreateContainerConfig.RestorePath.
//
// A context which ends only ends the wait of the client. A server since
// v1.1.0 finishes the create in a task of its own and keeps watching the
// container, which may exist although the call failed with the error of the
// context, so the caller deletes it with the runtime, and kills it first
// after a restore, as a restored container runs. A server since v1.2.0 kills
// a runtime which takes longer than 10 minutes to create the container (or to
// restore it, since v1.2.3) and fails the call.
func (c *ConmonClient) CreateContainer(
	ctx context.Context, cfg *CreateContainerConfig,
) (_ *CreateContainerResult, retErr error) {
	ctx, span := c.startSpan(ctx, "CreateContainer")
	defer endSpan(span, &retErr)

	if cfg == nil {
		return nil, ErrMissingConfig
	}

	driverTypes, err := validateLogDrivers(cfg.LogDrivers)
	if err != nil {
		return nil, err
	}

	// An older server would ignore the path and create a new container, so a
	// restore goes to the server which got checked for the feature.
	var feature, usage string
	if cfg.RestorePath != "" {
		feature, usage = FeatureRestore, "restoring a checkpoint"
	}

	fill := func(req proto.Conmon_CreateContainerRequest) error {
		if err := req.SetId(cfg.ID); err != nil {
			return fmt.Errorf("set ID: %w", err)
		}

		if err := req.SetBundlePath(cfg.BundlePath); err != nil {
			return fmt.Errorf("set bundle path: %w", err)
		}

		req.SetTerminal(cfg.Terminal)
		req.SetStdin(cfg.Stdin)

		if err := stringSliceToTextList(cfg.ExitPaths, req.NewExitPaths); err != nil {
			return fmt.Errorf("convert exit paths string slice to text list: %w", err)
		}

		if err := stringSliceToTextList(cfg.OOMExitPaths, req.NewOomExitPaths); err != nil {
			return fmt.Errorf("convert oom exit paths string slice to text list: %w", err)
		}

		if err := initLogDrivers(&req, cfg.LogDrivers, driverTypes); err != nil {
			return fmt.Errorf("init log drivers: %w", err)
		}

		if err := stringSliceToTextList(cfg.CommandArgs, req.NewCommandArgs); err != nil {
			return fmt.Errorf("convert command arguments string slice to text list: %w", err)
		}

		req.SetCgroupManager(proto.Conmon_CgroupManager(c.cgroupManager))

		if err := req.SetRestorePath(cfg.RestorePath); err != nil {
			return fmt.Errorf("set restore path: %w", err)
		}

		return nil
	}

	read := func(response proto.Conmon_CreateContainerResponse) (*CreateContainerResult, error) {
		return &CreateContainerResult{PID: response.ContainerPid()}, nil
	}

	return callRequiring(ctx, c, feature, usage, proto.Conmon.CreateContainer, fill, read)
}

// ExecSyncContainerConfig is the configuration for calling the
// ExecSyncContainer method.
type ExecSyncContainerConfig struct {
	// ID is the container identifier. Servers since v1.2.1 reject one which
	// CreateContainerConfig.ID does not allow.
	ID string

	// Command is a slice of command line arguments.
	Command []string

	// Timeout is the maximum time the command can run, rounded up to whole
	// seconds, zero for no limit. A timeout in seconds, like the one of the
	// CRI ExecSyncRequest, needs time.Duration(timeout) * time.Second: a
	// plain time.Duration(timeout) counts nanoseconds, which round up to one
	// second. A negative Timeout fails the call with an error matching
	// ErrInvalidValue, for example one computed from a deadline which
	// already passed. A deadline of the context limits it further, to the
	// whole seconds left minus one: the server answers once it killed the
	// command and collected the output still in flight, which takes up to
	// another second if a background process keeps writing it, so that it
	// usually reports the timeout before the context ends the call. With
	// less than two seconds left, the command gets the minimum of one second
	// the server needs, and may outlive the caller by up to a second. A
	// context which already ended fails the call without running the
	// command.
	//
	// A context which ends without a deadline, for example because it got
	// cancelled, only ends the call: the server does not stop the command,
	// which runs until it exits or reaches the Timeout. Set a Timeout or a
	// deadline to bound it.
	Timeout time.Duration

	// Terminal specifies if a tty should be used.
	Terminal bool
}

// ExecSyncContainerResult is the result for calling the ExecSyncContainer
// method.
type ExecSyncContainerResult struct {
	// ExitCode specifies the returned exit status, 128 plus the number of
	// the signal which killed the command, like a shell reports it, also
	// for real-time signals since v1.2.2, which gave -1 before. Since
	// v1.2.3, also a traced process, like one under strace, gets its
	// exit code instead of -1. It is ExecExitCodeNotStarted if the server
	// could not start the command, ExecExitCodeTimedOut if it timed out,
	// and ExecExitCodeUnknown if the server could not tell how the command
	// ended.
	ExitCode int32

	// Stdout contains the stdout stream result. Since v1.2.3, Stdout
	// and Stderr together hold at most 16 MiB less 64 KiB. Each keeps up to
	// half of it, and more if the other one leaves room, the rest gets
	// dropped. Before, each took up to 16 MiB.
	Stdout []byte

	// Stderr contains the stderr stream result, limited like Stdout.
	Stderr []byte

	// TimedOut is true if the command timed out.
	TimedOut bool
}

const (
	// ExecExitCodeUnknown is the exit code of an exec whose exit status the
	// server could not tell, because the command neither exited nor got
	// killed by a signal, or the wait for it failed. A server without pidfd
	// support reports a failed wait as ExecExitCodeTimedOut instead.
	ExecExitCodeUnknown int32 = -1

	// ExecExitCodeNotStarted is the exit code of an exec which the server
	// could not start, for example because the runtime failed or does not
	// know the container. Stderr tells why.
	ExecExitCodeNotStarted int32 = -2

	// ExecExitCodeTimedOut is the exit code of an exec which did not exit
	// before its timeout, TimedOut is set then. A server without pidfd
	// support reports it as well if it could not get the exit status of the
	// command, without TimedOut.
	ExecExitCodeTimedOut int32 = -3
)

// ExecSyncContainer can be used to execute a command within a running
// container. It returns once the command exited and its output got
// collected, see ExecSyncContainerConfig.Timeout for how long that may take.
// The command running but failing is no error, see
// ExecSyncContainerResult.ExitCode.
func (c *ConmonClient) ExecSyncContainer(
	ctx context.Context,
	cfg *ExecSyncContainerConfig,
) (_ *ExecSyncContainerResult, retErr error) {
	ctx, span := c.startSpan(ctx, "ExecSyncContainer")
	defer endSpan(span, &retErr)

	if cfg == nil {
		return nil, ErrMissingConfig
	}

	// It would mean no limit on the wire.
	if cfg.Timeout < 0 {
		return nil, fmt.Errorf("%w: exec timeout %v", ErrInvalidValue, cfg.Timeout)
	}

	// The command would get at least the minimum timeout of the server.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("exec sync: %w", err)
	}

	return call(ctx, c, proto.Conmon.ExecSyncContainer,
		func(req proto.Conmon_ExecSyncContainerRequest) error {
			if err := req.SetId(cfg.ID); err != nil {
				return fmt.Errorf("set ID: %w", err)
			}

			req.SetTimeoutSec(execTimeout(ctx, cfg.Timeout))

			if err := stringSliceToTextList(cfg.Command, req.NewCommand); err != nil {
				return fmt.Errorf("convert command string slice to text list: %w", err)
			}

			req.SetTerminal(cfg.Terminal)
			req.SetCgroupManager(proto.Conmon_CgroupManager(c.cgroupManager))

			return nil
		},
		func(resp proto.Conmon_ExecSyncContainerResponse) (*ExecSyncContainerResult, error) {
			stdout, err := resp.Stdout()
			if err != nil {
				return nil, fmt.Errorf("get stdout: %w", err)
			}

			stderr, err := resp.Stderr()
			if err != nil {
				return nil, fmt.Errorf("get stderr: %w", err)
			}

			return &ExecSyncContainerResult{
				ExitCode: resp.ExitCode(),
				// The release of the answer zeroes the slices.
				Stdout:   bytes.Clone(stdout),
				Stderr:   bytes.Clone(stderr),
				TimedOut: resp.TimedOut(),
			}, nil
		},
	)
}

// execTimeout returns the timeout of an exec in seconds, see
// ExecSyncContainerConfig.Timeout.
func execTimeout(ctx context.Context, timeout time.Duration) uint64 {
	seconds := timeoutSeconds(timeout)

	deadline, ok := ctx.Deadline()
	if !ok {
		return seconds
	}

	// The whole seconds left minus a margin, as the server answers only
	// after it killed the command and collected its output for up to a
	// second, so that it usually reports the timeout before the context of
	// the call ends. A short deadline still gets one second, 0 would mean
	// no timeout.
	const margin = 1

	secondsLeft := uint64(max(time.Until(deadline), 0) / time.Second)
	remaining := max(secondsLeft, margin+1) - margin

	if seconds == 0 {
		return remaining
	}

	return min(seconds, remaining)
}

// logDriverTypeToProto maps the log driver type to its protocol value.
func logDriverTypeToProto(typ LogDriverType) (proto.Conmon_LogDriver_Type, error) {
	switch typ {
	case LogDriverTypeContainerRuntimeInterface:
		return proto.Conmon_LogDriver_Type_containerRuntimeInterface, nil
	case LogDriverTypeJournald:
		return proto.Conmon_LogDriver_Type_journald, nil
	default:
		return 0, fmt.Errorf("%w: %v", ErrInvalidValue, typ)
	}
}

// validateLogDrivers checks the types of the log drivers before the request
// gets built, a validation error must not tear down the RPC connection, and
// returns their protocol values.
func validateLogDrivers(logDrivers []ContainerLogDriver) ([]proto.Conmon_LogDriver_Type, error) {
	types := make([]proto.Conmon_LogDriver_Type, 0, len(logDrivers))

	for _, logDriver := range logDrivers {
		typ, err := logDriverTypeToProto(logDriver.Type)
		if err != nil {
			return nil, fmt.Errorf("log driver type: %w", err)
		}

		types = append(types, typ)
	}

	return types, nil
}

// initLogDrivers sets the log drivers with the types which
// validateLogDrivers returned for them.
func initLogDrivers(
	req *proto.Conmon_CreateContainerRequest, logDrivers []ContainerLogDriver,
	types []proto.Conmon_LogDriver_Type,
) error {
	newLogDrivers, err := req.NewLogDrivers(int32(len(logDrivers)))
	if err != nil {
		return fmt.Errorf("create log drivers: %w", err)
	}

	for i, logDriver := range logDrivers {
		n := newLogDrivers.At(i)
		n.SetType(types[i])

		if err := n.SetPath(logDriver.Path); err != nil {
			return fmt.Errorf("set log driver path: %w", err)
		}

		n.SetMaxSize(logDriver.MaxSize)

		if err := n.SetContainerName(logDriver.ContainerName); err != nil {
			return fmt.Errorf("set log driver container name: %w", err)
		}
	}

	return nil
}

// ReopenLogContainerConfig is the configuration for calling the
// ReopenLogContainer method.
type ReopenLogContainerConfig struct {
	// ID is the container identifier. A request for a container which the
	// server does not know fails with an error matching
	// [ErrContainerNotFound].
	ID string
}

// ReopenLogContainer starts new log files at the paths of the log drivers of
// the container, after the caller rotated them, for example by renaming the
// old files like the kubelet does. It does not rotate the files itself. The
// journald driver is not affected.
func (c *ConmonClient) ReopenLogContainer(
	ctx context.Context,
	cfg *ReopenLogContainerConfig,
) (retErr error) {
	ctx, span := c.startSpan(ctx, "ReopenLogContainer")
	defer endSpan(span, &retErr)

	if cfg == nil {
		return ErrMissingConfig
	}

	_, err := call(ctx, c, proto.Conmon.ReopenLogContainer,
		func(req proto.Conmon_ReopenLogRequest) error {
			if err := req.SetId(cfg.ID); err != nil {
				return fmt.Errorf("set ID: %w", err)
			}

			return nil
		},
		noResponse,
	)

	return err
}
