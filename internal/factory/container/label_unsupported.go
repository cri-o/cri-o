//go:build !linux
// +build !linux

package container

func (slabel *secLabelImp) SecurityLabel(path string, seclabel string, shared, maybeRelabel bool) error {
	return nil
}
