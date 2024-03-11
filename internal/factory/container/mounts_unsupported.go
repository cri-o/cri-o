//go:build !linux
// +build !linux

package container

func (ctr *container) setMountLabel(mountLabel string) {
	return
}
