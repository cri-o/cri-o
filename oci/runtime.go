package oci

import (
	"bytes"
	"fmt"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/net/context"
)

// ExecSyncResponse is returned from ExecSync.
type ExecSyncResponse struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int32
}

// ExecSyncError wraps command's streams, exit code and error on ExecSync error.
type ExecSyncError struct {
	Stdout   bytes.Buffer
	Stderr   bytes.Buffer
	ExitCode int32
	Err      error
}

// Error fullfils the error interface in Golang.
func (e ExecSyncError) Error() string {
	return fmt.Sprintf("command error: %+v, stdout: %s, stderr: %s, exit code %d", e.Err, e.Stdout.Bytes(), e.Stderr.Bytes(), e.ExitCode)
}

// Runtime is an interface for runtimes supported in CRI-O.
// TODO(runcom): switch this to Runtime once ready
type RuntimeInterface interface {
	Name() string
	Path() string
	Version() (string, error)
	// TODO(runcom, move cgroupParent somewhere else...maybe?)
	CreateContainer(c *Container, cgroupParent string) error
	StartContainer(c *Container) error
	ExecSync(c *Container, command []string, timeout int64) (resp *ExecSyncResponse, err error)
	UpdateContainer(c *Container, res *rspec.LinuxResources) error
	WaitContainerStateStopped(ctx context.Context, c *Container) (err error)
	StopContainer(ctx context.Context, c *Container, timeout int64) error
	DeleteContainer(c *Container) error
	UpdateStatus(c *Container) error
	ContainerStatus(c *Container) *ContainerState
	PauseContainer(c *Container) error
	UnpauseContainer(c *Container) error
}
