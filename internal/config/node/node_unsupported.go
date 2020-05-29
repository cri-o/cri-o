// +build !linux

package node

import (
	"github.com/pkg/errors"
)

func ValidateConfig() error {
	return errors.Errorf("CRI-O is only supported on linux")
}
