// +build !linux

package sandbox

import (
	"fmt"
	"os"
)

func isNSorErr(nspath string) error {
	return fmt.Errorf("netns is not implemented for this platform")
}

func newNetNs() (*NetNs, error) {
	return &NetNs{}, fmt.Errorf("netns is not implemented for this platform")
}

func getNetNs(path string) (*NetNs, error) {
	return &NetNs{}, fmt.Errorf("netns is not implemented for this platform")
}

// NetNs handles data pertaining a network namespace
// for non-linux this is a noop
type NetNs struct {
	symlink *os.File
}

func (netns *NetNs) Path() string {
	return ""
}

func (netns *NetNs) symlinkCreate(name string) error {
	return nil
}

func (netns *NetNs) symlinkRemove() error {
	return nil
}

func (netns *NetNs) Close() error {
	return nil
}

func (netns *NetNs) Remove() error {
	return nil
}

func hostNetNsPath() (string, error) {
	return "", fmt.Errorf("netns is not implemented for this platform")
}
