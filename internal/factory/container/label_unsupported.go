//go:build !linux
// +build !linux

package container

func (slabel *secLabelImp) securityLabel(path string, seclabel string, shared, maybeRelabel bool) error {
	return nil
}
