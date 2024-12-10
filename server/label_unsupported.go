//go:build !linux

package server

func securityLabel(path, seclabel string, shared, maybeRelabel bool) error {
	return nil
}
