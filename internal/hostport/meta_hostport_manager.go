package hostport

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	v1 "k8s.io/api/core/v1"
	utilnet "k8s.io/utils/net"
	"sigs.k8s.io/knftables"

	utiliptables "github.com/cri-o/cri-o/internal/iptables"
)

// metaHostportManager is a HostPortManager that manages other HostPort managers internally.
type metaHostportManager struct {
	managers map[utilnet.IPFamily]*hostportManagers
}

type hostportManagers struct {
	iptables HostPortManager
	nftables HostPortManager
}

// NewMetaHostportManager creates a new HostPortManager.
func NewMetaHostportManager(ctx context.Context) (HostPortManager, error) {
	mh := &metaHostportManager{
		managers: make(map[utilnet.IPFamily]*hostportManagers),
	}

	iptv4, iptErr := newHostportManagerIPTables(ctx, utiliptables.ProtocolIPv4)
	nftv4, nftErr := newHostportManagerNFTables(knftables.IPv4Family)

	if iptv4 == nil && nftv4 == nil {
		return nil, fmt.Errorf("can't create HostPortManager: no support for iptables (%w) or nftables (%w)", iptErr, nftErr)
	}

	mh.managers[utilnet.IPv4] = &hostportManagers{
		iptables: iptv4,
		nftables: nftv4,
	}

	// IPv6 may fail if there's no kernel support, or no ip6tables binaries. We leave
	// mh.managers[utilnet.IPv6] nil if there's no IPv6 support.
	iptv6, iptErr := newHostportManagerIPTables(ctx, utiliptables.ProtocolIPv6)

	nftv6, nftErr := newHostportManagerNFTables(knftables.IPv6Family)

	switch {
	case nftv6 == nil:
		logrus.Infof("No kernel support for IPv6: %v", nftErr)
	case iptv6 == nil:
		logrus.Infof("No iptables support for IPv6: %v", iptErr)
	default:
		mh.managers[utilnet.IPv6] = &hostportManagers{
			iptables: iptv6,
			nftables: nftv6,
		}
	}

	return mh, nil
}

var netlinkFamily = map[utilnet.IPFamily]netlink.InetFamily{
	utilnet.IPv4: unix.AF_INET,
	utilnet.IPv6: unix.AF_INET6,
}

func (mh *metaHostportManager) Add(id, name, podIP string, hostportMappings []*PortMapping) error {
	family := utilnet.IPFamilyOfString(podIP)

	hostportMappings = filterHostportMappings(hostportMappings, family)
	if len(hostportMappings) == 0 {
		return nil
	}

	managers := mh.managers[family]
	if managers == nil {
		// No support for IPv6 but we got an IPv6 pod. This shouldn't happen.
		return fmt.Errorf("no HostPort support for IPv%s on this host", family)
	}

	// Use nftables if available, fall back to iptables. (We know at least one must be
	// non-nil.)
	hm := managers.nftables
	if hm == nil {
		hm = managers.iptables
	}

	err := hm.Add(id, name, podIP, hostportMappings)
	if err != nil {
		return err
	}

	// Remove conntrack entries just after adding the new iptables rules. If the
	// conntrack entry is removed along with the IP tables rule, it can be the case
	// that the packets received by the node after iptables rule removal will create a
	// new conntrack entry without any DNAT. That will result in blackhole of the
	// traffic even after correct iptables rules have been added back.
	conntrackPortsToRemove := []int{}

	for _, pm := range hostportMappings {
		if pm.Protocol == v1.ProtocolUDP {
			conntrackPortsToRemove = append(conntrackPortsToRemove, int(pm.HostPort))
		}
	}

	logrus.Infof("Deleting UDP conntrack entries for IPv%s: %v", family, conntrackPortsToRemove)

	for _, port := range conntrackPortsToRemove {
		err = deleteConntrackEntriesForDstPort(uint16(port), unix.IPPROTO_UDP, netlinkFamily[family])
		if err != nil {
			logrus.Errorf("Failed to clear udp conntrack for port %d, error: %v", port, err)
		}
	}

	return nil
}

func (mh *metaHostportManager) Remove(id string, hostportMappings []*PortMapping) error {
	var errstrings []string
	// Remove may not have the IP information, so we try to clean us much as possible
	// and warn about the possible errors. We also use both managers, if available,
	// to handle switching between iptables and nftables on upgrade/downgrade.

	for family, managers := range mh.managers {
		mappingsForFamily := filterHostportMappings(hostportMappings, family)
		if len(mappingsForFamily) == 0 {
			continue
		}

		if managers.nftables != nil {
			err := managers.nftables.Remove(id, mappingsForFamily)
			if err != nil {
				errstrings = append(errstrings, err.Error())
			}
		}

		if managers.iptables != nil {
			err := managers.iptables.Remove(id, mappingsForFamily)
			// Ignore iptables errors if we're primarily using nftables
			if err != nil && managers.nftables == nil {
				errstrings = append(errstrings, err.Error())
			}
		}
	}

	if len(errstrings) > 0 {
		return errors.New(strings.Join(errstrings, "\n"))
	}

	return nil
}

// filterHostportMappings returns only the PortMappings that apply to family.
func filterHostportMappings(hostportMappings []*PortMapping, family utilnet.IPFamily) []*PortMapping {
	mappings := []*PortMapping{}

	for _, pm := range hostportMappings {
		if pm.HostPort <= 0 {
			continue
		}

		if pm.HostIP != "" && utilnet.IPFamilyOfString(pm.HostIP) != family {
			continue
		}

		mappings = append(mappings, pm)
	}

	return mappings
}
