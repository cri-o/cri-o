//go:build !linux
// +build !linux

package server

func securityLabel(path string, seclabel string, shared, maybeRelabel bool) error {
	return nil
}
