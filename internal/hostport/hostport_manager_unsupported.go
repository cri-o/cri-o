//go:build !linux

package hostport

import (
	"fmt"
	"runtime"

	"github.com/vishvananda/netlink"
)

// deleteConntrackEntriesForDstPort delete the conntrack entries for the connections specified
// by the given destination port, protocol and IP family
func deleteConntrackEntriesForDstPort(port uint16, protocol uint8, family netlink.InetFamily) error {
	return fmt.Errorf("deleteConntrackEntriesForDstPort unsupported on %s", runtime.GOOS)
}
