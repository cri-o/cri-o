package hostport

import (
	"net"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
	iptablesproxy "k8s.io/kubernetes/pkg/proxy/iptables"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	utilexec "k8s.io/utils/exec"
)

// Manager is the main structure of this package.
type Manager struct {
	v4 hostport.HostPortManager
}

// New creates a new hostport.Manager instance.
func New() *Manager {
	iptInterface := utiliptables.New(utilexec.New(), utiliptables.ProtocolIPv4)
	if _, err := iptInterface.EnsureChain(
		utiliptables.TableNAT, iptablesproxy.KubeMarkMasqChain,
	); err != nil {
		logrus.Warnf("Unable to ensure iptables chain: %v", err)
	}
	v4 := hostport.NewHostportManager(iptInterface)

	return &Manager{v4}
}

// Add can be used to add a new hostport mapping to the provided sandbox.
func (m *Manager) Add(sb *sandbox.Sandbox, ip net.IP) (err error) {
	if sb == nil {
		return errors.New("sandbox is nil")
	}

	mapping := &hostport.PodPortMapping{
		Name:         sb.Name(),
		PortMappings: sb.PortMappings(),
		IP:           ip,
		HostNetwork:  false,
	}

	// use the corresponding IP family hostportManager for the IP
	if err := m.v4.Add(sb.ID(), mapping, "lo"); err != nil {
		return errors.Wrapf(
			err, "add hostport mapping for sandbox %s (%s)", sb.Name(), sb.ID(),
		)
	}

	return nil
}

// Remove can be used to delete a hostport mapping from the provided sandbox.
func (m *Manager) Remove(sb *sandbox.Sandbox) (err error) {
	if sb == nil {
		return errors.New("sandbox is nil")
	}

	mapping := &hostport.PodPortMapping{
		Name:         sb.Name(),
		PortMappings: sb.PortMappings(),
		HostNetwork:  false,
	}
	if err := m.v4.Remove(sb.ID(), mapping); err != nil {
		return errors.Wrapf(
			err,
			"remove hostport mapping for sandbox %s (%s)", sb.Name(), sb.ID(),
		)
	}

	return nil
}
