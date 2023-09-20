package client

import "github.com/containers/conmon-rs/internal/proto"

// LogLevel is the enum for all available server log levels.
type LogLevel string

const (
	// LogLevelTrace is the log level printing only "trace" messages.
	LogLevelTrace LogLevel = "trace"

	// LogLevelDebug is the log level printing only "debug" messages.
	LogLevelDebug LogLevel = "debug"

	// LogLevelInfo is the log level printing only "info" messages.
	LogLevelInfo LogLevel = "info"

	// LogLevelWarn is the log level printing only "warn" messages.
	LogLevelWarn LogLevel = "warn"

	// LogLevelError is the log level printing only "error" messages.
	LogLevelError LogLevel = "error"

	// LogLevelOff is the log level printing no messages.
	LogLevelOff LogLevel = "off"
)

// LogDriver is the enum for all available server log drivers.
type LogDriver string

const (
	// LogDriverStdout is the log driver printing to stdio.
	LogDriverStdout LogDriver = "stdout"

	// LogDriverSystemd is the log driver printing to systemd journald.
	LogDriverSystemd LogDriver = "systemd"
)

// CgroupManager is the enum for all available cgroup managers.
type CgroupManager proto.Conmon_CgroupManager

const (
	// CgroupManagerSystemd specifies to use systemd to create and manage
	// cgroups.
	CgroupManagerSystemd CgroupManager = CgroupManager(proto.Conmon_CgroupManager_systemd)

	// CgroupManagerCgroupfs specifies to use the cgroup filesystem to create
	// and manage cgroups.
	CgroupManagerCgroupfs CgroupManager = CgroupManager(proto.Conmon_CgroupManager_cgroupfs)

	// CgroupManagerPerCommand opts-in to the new CgroupManager option specified at the command level.
	//
	// Set `ConmonServerConfig.CgroupManager` to `CgroupManagerPerCommand` to use
	// the CgroupManager specified in the command config (e.g. CreateContainerConfig).
	CgroupManagerPerCommand CgroupManager = CgroupManager(0xffff)
)

// Namespace is the enum for all available namespaces.
type Namespace int

const (
	// NamespaceIPC is the reference to the IPC namespace.
	NamespaceIPC Namespace = iota

	// NamespacePID is the reference to the PID namespace.
	NamespacePID

	// NamespaceNet is the reference to the network namespace.
	NamespaceNet

	// NamespaceUser is the reference to the user namespace.
	NamespaceUser

	// NamespaceUTS is the reference to the UTS namespace.
	NamespaceUTS
)
