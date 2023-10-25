//go:build !linux
// +build !linux

package sandbox

import (
	"context"
	"fmt"
	"os"
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
	symlink *os.File
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
