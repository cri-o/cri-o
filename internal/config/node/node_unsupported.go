//go:build !linux
// +build !linux

package node

import "fmt"

func ValidateConfig() error {
	return fmt.Errorf("CRI-O is only supported on linux")
}
