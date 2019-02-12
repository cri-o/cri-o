// +build !exclude_graphdriver_lvm,linux

package register

import (
	// register lvm
	_ "github.com/containers/storage/drivers/lvm"
)
