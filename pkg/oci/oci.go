package oci

import (
	"io"
	"syscall"

	spec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
	"k8s.io/client-go/tools/remotecommand"
)

// RuntimeImpl is an interface used by the caller to interact with the
// container runtime. The purpose of this interface being to abstract
// implementations and their associated assumptions regarding the way to
// interact with containers. This will allow for new implementations of
// this interface, especially useful for the case of VM based container
// runtimes. Assumptions based on the fact that a container process runs
// on the host will be limited to the RuntimeOCI implementation.
type RuntimeImpl interface {
	CreateContainer(*Container, string) error
	StartContainer(*Container) error
	ExecContainer(*Container, []string, io.Reader, io.WriteCloser, io.WriteCloser,
		bool, <-chan remotecommand.TerminalSize) error
	ExecSyncContainer(*Container, []string, int64) (*ExecSyncResponse, error)
	UpdateContainer(*Container, *spec.LinuxResources) error
	StopContainer(context.Context, *Container, int64) error
	DeleteContainer(*Container) error
	UpdateContainerStatus(*Container) error
	PauseContainer(*Container) error
	UnpauseContainer(*Container) error
	ContainerStats(*Container) (*ContainerStats, error)
	SignalContainer(*Container, syscall.Signal) error
	AttachContainer(*Container, io.Reader, io.WriteCloser, io.WriteCloser, bool, <-chan remotecommand.TerminalSize) error
	PortForwardContainer(*Container, int32, io.ReadWriter) error
	ReopenContainerLog(*Container) error
	WaitContainerStateStopped(context.Context, *Container) error
}

// ContainerStats contains the statistics information for a running container
type ContainerStats struct {
	Container   string
	CPU         float64
	CPUNano     uint64
	SystemNano  int64
	MemUsage    uint64
	MemLimit    uint64
	MemPerc     float64
	NetInput    uint64
	NetOutput   uint64
	BlockInput  uint64
	BlockOutput uint64
	PIDs        uint64
}

// ExecSyncResponse is returned from ExecSync.
type ExecSyncResponse struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int32
}
