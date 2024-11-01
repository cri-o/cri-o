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

func (mh *metaHostportManager) Add(id string, podPortMapping *PodPortMapping) error {
	if utilnet.IsIPv6(podPortMapping.IP) {
		return mh.ipv6HostportManager.Add(id, podPortMapping)
	}

	return mh.ipv4HostportManager.Add(id, podPortMapping)
}

func (mh *metaHostportManager) Remove(id string, podPortMapping *PodPortMapping) error {
	var errstrings []string
	// Remove may not have the IP information, so we try to clean us much as possible
	// and warn about the possible errors
	err := mh.ipv4HostportManager.Remove(id, podPortMapping)
	if err != nil {
		errstrings = append(errstrings, err.Error())
	}

	err = mh.ipv6HostportManager.Remove(id, podPortMapping)
	if err != nil {
		errstrings = append(errstrings, err.Error())
	}

	if len(errstrings) > 0 {
		return errors.New(strings.Join(errstrings, "\n"))
	}

	return nil
}
