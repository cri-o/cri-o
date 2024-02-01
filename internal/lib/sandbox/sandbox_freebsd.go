package sandbox

import (
	"context"
	"fmt"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (n *NetNs) Initialize() (*NetNs, error) {
	return &NetNs{}, fmt.Errorf("netns is not implemented for this platform")
}

func (n *NetNs) Initialized(nsType string) bool {
	return false
}

func getNetNs(path string) (*NetNs, error) {
	return &NetNs{}, fmt.Errorf("netns is not implemented for this platform")
}

// NetNs handles data pertaining a network namespace
// for non-linux this is a noop
type NetNs struct {
}

func (n *NetNs) Get() *NetNs {
	return n
}

func (n *NetNs) Path() string {
	return ""
}

func (n *NetNs) SymlinkCreate(name string) error {
	return nil
}

func (n *NetNs) symlinkRemove() error {
	return nil
}

func (n *NetNs) Close() error {
	return nil
}

func (n *NetNs) Remove() error {
	return nil
}

func hostNetNsPath() (string, error) {
	return "", fmt.Errorf("netns is not implemented for this platform")
}

// UnmountShm removes the shared memory mount for the sandbox and returns an
// error if any failure occurs.
func (s *Sandbox) UnmountShm(ctx context.Context) error {
	return nil
}

// NeedsInfra is a function that returns whether the sandbox will need an infra
// container. On FreeBSD, if any supported namespace mode is pod, we need a
// parent jail to own the namespaces. To start with, we only support pod mode
// for network but in future could add ipc.
func (s *Sandbox) NeedsInfra(serverDropsInfra bool) bool {
	return !serverDropsInfra || s.nsOpts.Network == types.NamespaceMode_POD
}
