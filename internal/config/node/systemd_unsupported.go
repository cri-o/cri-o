//go:build !linux
// +build !linux

package node

func SystemdHasCollectMode() bool {
	return false
}

func SystemdHasAllowedCPUs() bool {
	return false
}

// systemdSupportsProperty checks whether systemd supports a property
// It returns an error if it does not.
func systemdSupportsProperty(property string) (bool, error) {
	return false, nil
}
