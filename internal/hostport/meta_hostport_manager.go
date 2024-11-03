package hostport

import (
	"context"
	"errors"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	v1 "k8s.io/api/core/v1"
	utilexec "k8s.io/utils/exec"
	utilnet "k8s.io/utils/net"

	utiliptables "github.com/cri-o/cri-o/internal/iptables"
)

type metaHostportManager struct {
	ipv4HostportManager HostPortManager
	ipv6HostportManager HostPortManager
}

// NewMetaHostportManager creates a new HostPortManager.
func NewMetaHostportManager(ctx context.Context) HostPortManager {
	exec := utilexec.New()
	// Create IPv4 handler
	iptInterface := utiliptables.New(ctx, exec, utiliptables.ProtocolIPv4)
	hostportManagerv4 := NewHostportManager(iptInterface)
	// Create IPv6 handler
	ip6tInterface := utiliptables.New(ctx, exec, utiliptables.ProtocolIPv6)
	hostportManagerv6 := NewHostportManager(ip6tInterface)

	h := &metaHostportManager{
		ipv4HostportManager: hostportManagerv4,
		ipv6HostportManager: hostportManagerv6,
	}

	return h
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

	var err error
	if family == utilnet.IPv6 {
		err = mh.ipv6HostportManager.Add(id, name, podIP, hostportMappings)
	} else {
		err = mh.ipv4HostportManager.Add(id, name, podIP, hostportMappings)
	}

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
	// and warn about the possible errors

	hostportMappingsV4 := filterHostportMappings(hostportMappings, utilnet.IPv4)
	if len(hostportMappingsV4) > 0 {
		err := mh.ipv4HostportManager.Remove(id, hostportMappingsV4)
		if err != nil {
			errstrings = append(errstrings, err.Error())
		}
	}

	hostportMappingsV6 := filterHostportMappings(hostportMappings, utilnet.IPv6)
	if len(hostportMappingsV6) > 0 {
		err := mh.ipv6HostportManager.Remove(id, hostportMappingsV6)
		if err != nil {
			errstrings = append(errstrings, err.Error())
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
