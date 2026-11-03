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
	// starts with "-", which the runtime would take for an option.
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
	// -1 before. The paths have to be absolute, servers since v1.2.1 reject
	// other ones.
	ExitPaths []string

	// OOMExitPaths is a slice of files that should be created if the given
	// container is OOM killed. They are created empty before the exit files
	// get written. The paths have to be absolute, servers since v1.2.1
	// reject other ones.
	OOMExitPaths []string

	// LogDrivers is a slice of selected log drivers.
	LogDrivers []ContainerLogDriver

	// CleanupCmd is the command that will be executed once the container
	// exited and its exit files got written. The server does not wait for
	// it.
	CleanupCmd []string

	// GlobalArgs are the additional arguments passed to the create runtime call
	// before the command. e.g: crun --runtime-arg create
	GlobalArgs []string

	// CommandArgs are the additional arguments passed to the create runtime call
	// after the command. e.g: crun create --runtime-opt
	CommandArgs []string

	// EnvVars are the environment variables passed to the create runtime call.
	EnvVars map[string]string

	// CgroupManager can be used to select the cgroup manager.
	//
	// To use this option set [ConmonServerConfig.CgroupManager] to
	// [CgroupManagerPerCommand].
	CgroupManager CgroupManager

	// AdditionalFDs can be used to pass additional file descriptors to the
	// container. They are slots of the fd socket, see RemoteFDs, which the
	// runtime gets as fd 3 and onwards in their order. The runtime has to be
	// told to pass them on, for example with --preserve-fds in CommandArgs.
	AdditionalFDs []RemoteFD

	// LeakFDs can be used to keep file descriptors open as long as the
	// container is running. They are slots of the fd socket, see RemoteFDs,
	// which the server holds until the container exited without passing them
	// on.
	LeakFDs []RemoteFD

	// SdNotifySocket is the path of the host sd_notify socket, usually the
	// NOTIFY_SOCKET of systemd: an absolute path, or @name for an abstract
	// socket. Since v1.2.2, a relative path gets rejected. If set, the
	// server binds <BundlePath>/notify/notify.sock before it runs the
	// runtime and relays the messages sent there to the host socket until
	// the container exits.
	// Like conmon, it only passes the assignments which tell the state of a
	// service, like READY=1 or WATCHDOG=1, the sdnotifySocket field of
	// conmon.capnp in the repository lists them. The caller mounts the
	// notify directory into the container and sets NOTIFY_SOCKET for the
	// container process accordingly. Servers without [FeatureSdNotify]
	// would ignore it, CreateContainer fails with [ErrUnsupported] on them
	// instead.
	SdNotifySocket string
}

// ContainerLogDriver specifies a selected logging mechanism.
type ContainerLogDriver struct {
	// Type defines the log driver variant.
	Type LogDriverType

	// Path specifies the absolute filesystem path of the log driver.
	// Servers since v1.2.1 reject other ones.
	Path string

	// MaxSize is the maximum amount of bytes to be written before rotation.
	// 0 translates to an unlimited size.
	MaxSize uint64

	// ContainerName is written as CONTAINER_NAME by LogDriverTypeJournald,
	// like the name of conmon. Servers without FeatureJournaldFields ignore
	// it.
	ContainerName string

	// Tag is written as CONTAINER_TAG and SYSLOG_IDENTIFIER by
	// LogDriverTypeJournald, like the log tag of conmon. Without one, servers
	// since v1.2.2 use the ContainerName as the identifier, and the short
	// container ID without both, like conmon, older ones "conmonrs". Servers
	// without FeatureJournaldFields ignore it.
	Tag string

	// Labels are additional fields of every entry of LogDriverTypeJournald,
	// like the log labels of conmon. A key has to be a journal field name
	// like POD_NAME, the labels field of conmon.capnp in the repository has
	// the rules and limits, which the server checks. Servers without
	// [FeatureJournaldFields] ignore them.
	Labels map[string]string
}

// LogDriverType specifies available log drivers.
type LogDriverType int

const (
	// LogDriverTypeContainerRuntimeInterface is the Kubernetes CRI logger
	// type.
	LogDriverTypeContainerRuntimeInterface LogDriverType = iota

	// LogDriverTypeJSONLogger writes one JSON object per line to the path
	// of the driver.
	LogDriverTypeJSONLogger

	// LogDriverTypeJournald writes to the systemd journal, the path of the
	// driver is not used.
	LogDriverTypeJournald
)

// CreateContainerResponse is the response of the CreateContainer method.
type CreateContainerResponse struct {
	// PID is the container process identifier.
	PID uint32

	// NamespacesPath is the base path where the namespaces are mounted.
	//
	// Deprecated: The server never reported it, so it is always empty. Use
	// the paths returned by CreateNamespaces instead.
	NamespacesPath string
}

// CreateContainer creates a container by running the create command of the
// runtime and monitors it from then on. The container process waits for the
// start command of the runtime, which the caller runs, before it runs the
// command of the container. The server writes the exit and OOM files and runs
// the cleanup command once the container exited.
func (c *ConmonClient) CreateContainer(
	ctx context.Context, cfg *CreateContainerConfig,
) (_ *CreateContainerResponse, retErr error) {
	ctx, span := c.startSpan(ctx, "CreateContainer")
	defer endSpan(span, &retErr)

	if cfg == nil {
		return nil, ErrMissingConfig
	}

	if err := c.validateCgroupManager(cfg.CgroupManager); err != nil {
		return nil, err
	}

	driverTypes, err := validateLogDrivers(cfg.LogDrivers)
	if err != nil {
		return nil, err
	}

	// An older server ignores the field, the unit would never get ready.
	// Checked after the configuration, which needs no server, the request
	// goes to the checked server.
	var rpcClient proto.Conmon

	if cfg.SdNotifySocket != "" {
		rpcClient, err = c.requireServerFeature(ctx, FeatureSdNotify, "sd_notify socket")
	} else if rpcClient, err = c.client(ctx); err != nil {
		err = fmt.Errorf("create RPC client: %w", err)
	}

	if err != nil {
		return nil, err
	}

	defer rpcClient.Release()

	return callOn(ctx, c, rpcClient, proto.Conmon.CreateContainer,
		func(req proto.Conmon_CreateContainerRequest) error {
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

			if err := stringSliceToTextList(cfg.CleanupCmd, req.NewCleanupCmd); err != nil {
				return fmt.Errorf("convert cleanup command string slice to text list: %w", err)
			}

			if err := stringSliceToTextList(cfg.GlobalArgs, req.NewGlobalArgs); err != nil {
				return fmt.Errorf("convert global arguments string slice to text list: %w", err)
			}

			if err := stringSliceToTextList(cfg.CommandArgs, req.NewCommandArgs); err != nil {
				return fmt.Errorf("convert command arguments string slice to text list: %w", err)
			}

			if err := stringStringMapToMapEntryList(cfg.EnvVars, req.NewEnvVars); err != nil {
				return fmt.Errorf("convert environment variables to map entry list: %w", err)
			}

			c.setCgroupManager(cfg.CgroupManager, req)

			if err := remoteFDSliceToUInt64List(
				cfg.AdditionalFDs,
				req.NewAdditionalFds,
			); err != nil {
				return fmt.Errorf("convert file descriptor slice to slot list: %w", err)
			}

			if err := remoteFDSliceToUInt64List(cfg.LeakFDs, req.NewLeakFds); err != nil {
				return fmt.Errorf("convert file descriptor slice to slot list: %w", err)
			}

			if err := req.SetSdnotifySocket(cfg.SdNotifySocket); err != nil {
				return fmt.Errorf("set sd_notify socket: %w", err)
			}

			return nil
		},
		func(response proto.Conmon_CreateContainerResponse) (*CreateContainerResponse, error) {
			return &CreateContainerResponse{PID: response.ContainerPid()}, nil
		},
	)
}

// ExecSyncConfig is the configuration for calling the ExecSyncContainer
// method.
type ExecSyncConfig struct {
	// ID is the container identifier. Servers since v1.2.1 reject one which
	// CreateContainerConfig.ID does not allow.
	ID string

	// Command is a slice of command line arguments.
	Command []string

	// Timeout is the maximum time the command can run in seconds, 0 for no
	// limit. A deadline of the context limits it further, to the whole
	// seconds left minus one: the server answers once it killed the command
	// and collected the output still in flight, which takes up to another
	// second if a background process keeps writing it, so that it usually
	// reports the timeout before the context ends the call. With less than
	// two seconds left, the command gets the minimum of one second the
	// server needs, and may outlive the caller by up to a second. A context
	// which already ended fails the call without running the command.
	//
	// A context which ends without a deadline, for example because it got
	// cancelled, only ends the call: the server does not stop the command,
	// which runs until it exits or reaches the Timeout. Set a Timeout or a
	// deadline to bound it.
	Timeout uint64

	// Terminal specifies if a tty should be used.
	Terminal bool

	// EnvVars are the environment variables passed to the exec runtime call.
	EnvVars map[string]string

	// CgroupManager can be used to select the cgroup manager.
	//
	// To use this option set [ConmonServerConfig.CgroupManager] to
	// [CgroupManagerPerCommand].
	CgroupManager CgroupManager
}

// ExecContainerResult is the result for calling the ExecSyncContainer method.
type ExecContainerResult struct {
	// ExitCode specifies the returned exit status, 128 plus the number of
	// the signal which killed the command, like a shell reports it, also
	// for real-time signals since v1.2.2, which gave -1 before. It is
	// ExecExitCodeNotStarted if the server could not start the command,
	// ExecExitCodeTimedOut if it timed out, and -1 if the server could not
	// tell how the command ended.
	ExitCode int32

	// Stdout contains the stdout stream result.
	Stdout []byte

	// Stderr contains the stderr stream result.
	Stderr []byte

	// TimedOut is true if the command timed out.
	TimedOut bool
}

const (
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
// collected, see ExecSyncConfig.Timeout for how long that may take. The
// command running but failing is no error, see ExecContainerResult.ExitCode.
func (c *ConmonClient) ExecSyncContainer(
	ctx context.Context,
	cfg *ExecSyncConfig,
) (_ *ExecContainerResult, retErr error) {
	ctx, span := c.startSpan(ctx, "ExecSyncContainer")
	defer endSpan(span, &retErr)

	if cfg == nil {
		return nil, ErrMissingConfig
	}

	if err := c.validateCgroupManager(cfg.CgroupManager); err != nil {
		return nil, err
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

			if err := stringStringMapToMapEntryList(cfg.EnvVars, req.NewEnvVars); err != nil {
				return fmt.Errorf("convert environment variables to map entry list: %w", err)
			}

			c.setCgroupManager(cfg.CgroupManager, req)

			return nil
		},
		func(resp proto.Conmon_ExecSyncContainerResponse) (*ExecContainerResult, error) {
			stdout, err := resp.Stdout()
			if err != nil {
				return nil, fmt.Errorf("get stdout: %w", err)
			}

			stderr, err := resp.Stderr()
			if err != nil {
				return nil, fmt.Errorf("get stderr: %w", err)
			}

			return &ExecContainerResult{
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
// ExecSyncConfig.Timeout.
func execTimeout(ctx context.Context, timeout uint64) uint64 {
	deadline, ok := ctx.Deadline()
	if !ok {
		return timeout
	}

	// The whole seconds left minus a margin, as the server answers only
	// after it killed the command and collected its output for up to a
	// second, so that it usually reports the timeout before the context of
	// the call ends. A short deadline still gets one second, 0 would mean
	// no timeout.
	const margin = 1

	secondsLeft := uint64(max(time.Until(deadline), 0) / time.Second)
	remaining := max(secondsLeft, margin+1) - margin

	if timeout == 0 {
		return remaining
	}

	return min(timeout, remaining)
}

// logDriverTypes maps the log driver types to their protocol values.
var logDriverTypes = enumTable[LogDriverType, proto.Conmon_LogDriver_Type]{
	{LogDriverTypeContainerRuntimeInterface, proto.Conmon_LogDriver_Type_containerRuntimeInterface},
	{LogDriverTypeJSONLogger, proto.Conmon_LogDriver_Type_json},
	{LogDriverTypeJournald, proto.Conmon_LogDriver_Type_journald},
}

// validateLogDrivers checks the types of the log drivers before the request
// gets built, a validation error must not tear down the RPC connection, and
// returns their protocol values.
func validateLogDrivers(logDrivers []ContainerLogDriver) ([]proto.Conmon_LogDriver_Type, error) {
	types := make([]proto.Conmon_LogDriver_Type, 0, len(logDrivers))

	for _, logDriver := range logDrivers {
		typ, err := logDriverTypes.toProto(logDriver.Type)
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

		if err := n.SetTag(logDriver.Tag); err != nil {
			return fmt.Errorf("set log driver tag: %w", err)
		}

		if err := stringStringMapToMapEntryList(logDriver.Labels, n.NewLabels); err != nil {
			return fmt.Errorf("set log driver labels: %w", err)
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
