package hostport

import (
	"fmt"

	"github.com/vishvananda/netlink"
)

// deleteConntrackEntriesForDstPort delete the conntrack entries for the connections specified
// by the given destination port, protocol and IP family.
func deleteConntrackEntriesForDstPort(port uint16, protocol uint8, family netlink.InetFamily) error {
	filter := &netlink.ConntrackFilter{}
	err := filter.AddProtocol(protocol)
	if err != nil {
		return fmt.Errorf("error deleting connection tracking state for protocol: %d Port: %d, error: %w", protocol, port, err)
	}
	err = filter.AddPort(netlink.ConntrackOrigDstPort, port)
	if err != nil {
		return fmt.Errorf("error deleting connection tracking state for protocol: %d Port: %d, error: %w", protocol, port, err)
	}

	_, err = netlink.ConntrackDeleteFilters(netlink.ConntrackTable, family, filter)
	if err != nil {
		return fmt.Errorf("error deleting connection tracking state for protocol: %d Port: %d, error: %w", protocol, port, err)
	}
	return nil
}
