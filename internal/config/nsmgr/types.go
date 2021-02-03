package nsmgr

// NSType is an abstraction about available namespace types
type NSType string

const (
	NETNS         NSType = "net"
	IPCNS         NSType = "ipc"
	UTSNS         NSType = "uts"
	USERNS        NSType = "user"
	PIDNS         NSType = "pid"
	NumNamespaces        = 4
)

// SupportedNamespacesForPinning returns a slice of
// the names of namespaces that CRI-O supports
// pinning.
func SupportedNamespacesForPinning() []NSType {
	return []NSType{NETNS, IPCNS, UTSNS, USERNS}
}
