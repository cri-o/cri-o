//go:build !linux
// +build !linux

package server

func securityLabel(path, seclabel string, shared, maybeRelabel bool) error {
	return nil
}
