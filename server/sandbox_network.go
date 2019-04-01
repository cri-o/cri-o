package server

import (
	"fmt"
	"net"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	cnicurrent "github.com/containernetworking/cni/pkg/types/current"
	"github.com/cri-o/cri-o/lib/sandbox"
	"github.com/sirupsen/logrus"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

// networkStart sets up the sandbox's network and returns the pod IP on success
// or an error
func (s *Server) networkStart(sb *sandbox.Sandbox) (podIP string, result cnitypes.Result, err error) {
	if sb.HostNetwork() {
		return s.hostIP, nil, nil
	}

	// Ensure network resources are cleaned up if the plugin succeeded
	// but an error happened between plugin success and the end of networkStart()
	defer func() {
		if err != nil {
			s.networkStop(sb)
		}
	}()

	podNetwork := newPodNetwork(sb)
	_, err = s.netPlugin.SetUpPod(podNetwork)
	if err != nil {
		err = fmt.Errorf("failed to create pod network sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
		return
	}

	results, err := s.netPlugin.GetPodNetworkStatus(podNetwork)
	if err != nil {
		err = fmt.Errorf("failed to get network status for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
		return
	}

	podIP, result, err = s.getSandboxIPAndResult(results)
	if err != nil {
		err = fmt.Errorf("failed to get ip for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
		return
	}

	if len(sb.PortMappings()) > 0 {
		ip := net.ParseIP(podIP)
		if ip == nil {
			err = fmt.Errorf("failed to get valid ip address for sandbox %s(%s)", sb.Name(), sb.ID())
			return
		}

		err = s.hostportManager.Add(sb.ID(), &hostport.PodPortMapping{
			Name:         sb.Name(),
			PortMappings: sb.PortMappings(),
			IP:           ip,
			HostNetwork:  false,
		}, "lo")
		if err != nil {
			err = fmt.Errorf("failed to add hostport mapping for sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
			return
		}

	}
	return podIP, result, err
}

// getSandboxIP retrieves the IP address for the sandbox
func (s *Server) getSandboxIP(sb *sandbox.Sandbox) (string, error) {
	if sb.HostNetwork() {
		return s.hostIP, nil
	}

	podNetwork := newPodNetwork(sb)
	results, err := s.netPlugin.GetPodNetworkStatus(podNetwork)
	if err != nil {
		return "", fmt.Errorf("failed to get network status for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
	}

	ip, _, err := s.getSandboxIPAndResult(results)
	if err != nil {
		return "", fmt.Errorf("failed to get ip for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
	}
	return ip, nil
}

// getSandboxIPAndResult gets the sandbox ip from a list of results
// It chooses the first result in the list (lexicographic order)
func (s *Server) getSandboxIPAndResult(results []cnitypes.Result) (string, cnitypes.Result, error) {
	if len(results) == 0 {
		return "", nil, fmt.Errorf("no CNI config results network status")
	}
	result := results[0]
	// Choose the first result, as it should be the first in lexiconographic order
	network, err := cnicurrent.GetResult(result)
	if err != nil {
		return "", nil, err
	}

	if len(network.IPs) == 0 {
		return "", nil, fmt.Errorf("no IPs returned from CNI result")
	}

	return network.IPs[0].Address.IP.String(), result, nil
}

// networkStop cleans up and removes a pod's network.  It is best-effort and
// must call the network plugin even if the network namespace is already gone
func (s *Server) networkStop(sb *sandbox.Sandbox) {
	if sb.HostNetwork() {
		return
	}

	if err := s.hostportManager.Remove(sb.ID(), &hostport.PodPortMapping{
		Name:         sb.Name(),
		PortMappings: sb.PortMappings(),
		HostNetwork:  false,
	}); err != nil {
		logrus.Warnf("failed to remove hostport for pod sandbox %s(%s): %v",
			sb.Name(), sb.ID(), err)
	}

	podNetwork := newPodNetwork(sb)
	if err := s.netPlugin.TearDownPod(podNetwork); err != nil {
		logrus.Warnf("failed to destroy network for pod sandbox %s(%s): %v",
			sb.Name(), sb.ID(), err)
	}
}
