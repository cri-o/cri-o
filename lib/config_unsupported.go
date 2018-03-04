// +build !linux

package lib

func selinuxEnabled() bool {
	return false
}
