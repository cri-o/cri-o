//go:build !linux

package nsmgr

// IsShadowedMountError returns true if the error indicates a shadowed mount.
// On non-Linux platforms, this always returns false.
func IsShadowedMountError(err error) bool {
	return false
}
