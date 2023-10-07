//go:build !linux
// +build !linux

package container

func SecurityLabel(path string, seclabel string, shared, maybeRelabel bool) error {
	return nil
}
