package client

import (
	"strconv"

	"github.com/containers/conmon-rs/internal/proto"
)

// LogLevel is the enum for all available server log levels. Each level
// includes the messages of the levels after it, trace logs everything.
type LogLevel string

const (
	// LogLevelTrace is the log level printing all messages, including the
	// "trace" ones.
	LogLevelTrace LogLevel = "trace"

	// LogLevelDebug is the log level printing "debug", "info", "warn" and
	// "error" messages.
	LogLevelDebug LogLevel = "debug"

	// LogLevelInfo is the log level printing "info", "warn" and "error"
	// messages.
	LogLevelInfo LogLevel = "info"

	// LogLevelWarn is the log level printing "warn" and "error" messages.
	LogLevelWarn LogLevel = "warn"

	// LogLevelError is the log level printing only "error" messages.
	LogLevelError LogLevel = "error"

	// LogLevelOff is the log level printing no messages.
	LogLevelOff LogLevel = "off"
)

// LogDriver is the enum for all available server log drivers.
type LogDriver string

const (
	// LogDriverNone disables logging. Since v1.2.2, the message of a
	// panic still goes to the journal.
	LogDriverNone LogDriver = "none"

	// LogDriverStdout is the log driver printing to stdio.
	LogDriverStdout LogDriver = "stdout"

	// LogDriverSystemd is the log driver printing to systemd journald.
	LogDriverSystemd LogDriver = "systemd"
)

// CgroupManager is the enum for all available cgroup managers. The zero value
// is CgroupManagerSystemd.
type CgroupManager uint16

const (
	// CgroupManagerSystemd specifies to use systemd to create and manage
	// cgroups. It is the zero value, which an unset field selects.
	CgroupManagerSystemd CgroupManager = CgroupManager(proto.Conmon_CgroupManager_systemd)

	// CgroupManagerCgroupfs specifies to use the cgroup filesystem to create
	// and manage cgroups.
	CgroupManagerCgroupfs CgroupManager = CgroupManager(proto.Conmon_CgroupManager_cgroupfs)

	// CgroupManagerPerCommand selects the cgroup manager per command: set
	// [ConmonServerConfig.CgroupManager] to it to use the CgroupManager of
	// the configuration of each command, like
	// [CreateContainerConfig.CgroupManager].
	CgroupManagerPerCommand CgroupManager = CgroupManager(0xffff)
)

// String returns the name of the cgroup manager, "systemd", "cgroupfs" or
// "per command", and CgroupManager(<number>) for an unknown value.
func (m CgroupManager) String() string {
	switch m {
	case CgroupManagerSystemd:
		return "systemd"
	case CgroupManagerCgroupfs:
		return "cgroupfs"
	case CgroupManagerPerCommand:
		return "per command"
	default:
		return "CgroupManager(" + strconv.FormatUint(uint64(m), 10) + ")"
	}
}

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
