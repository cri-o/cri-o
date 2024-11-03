package hostport

import (
	"context"
	"errors"
	"strings"

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

func (mh *metaHostportManager) Add(id, name, podIP string, hostportMappings []*PortMapping) error {
	family := utilnet.IPFamilyOfString(podIP)

	hostportMappings = filterHostportMappings(hostportMappings, family)
	if len(hostportMappings) == 0 {
		return nil
	}

	if family == utilnet.IPv6 {
		return mh.ipv6HostportManager.Add(id, name, podIP, hostportMappings)
	}

	return mh.ipv4HostportManager.Add(id, name, podIP, hostportMappings)
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
