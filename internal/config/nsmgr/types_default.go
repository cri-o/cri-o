//go:build !linux

package nsmgr

// IsInvalidNamespaceMountError returns true if the error indicates an invalid namespace mount.
// On non-Linux platforms, this always returns false.
func IsInvalidNamespaceMountError(err error) bool {
	return false
}
