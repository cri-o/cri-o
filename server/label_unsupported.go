// +build !linux

package server

func securityLabel(path string, seclabel string, shared bool) error {
	return nil
}
