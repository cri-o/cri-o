//go:build !linux
// +build !linux

package container

func (label *SecLabel) SecurityLabel(path string, seclabel string, shared, maybeRelabel bool) error {
	return nil
}
