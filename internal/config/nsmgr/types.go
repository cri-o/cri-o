package nsmgr

// NSType is a representation of available namespace types.
type NSType string

const (
	NETNS                NSType = "net"
	IPCNS                NSType = "ipc"
	UTSNS                NSType = "uts"
	USERNS               NSType = "user"
	PIDNS                NSType = "pid"
	ManagedNamespacesNum        = 5
)

// Namespace provides a generic namespace interface.
type Namespace interface {
	// Path returns the bind mount path of the namespace.
	Path() string

	// Type returns the namespace type (net, ipc, user, pid or uts).
	Type() NSType

	// Remove ensures this namespace is closed and removed.
	Remove() error
}
