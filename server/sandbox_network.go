package server

import (
	"context"
	"fmt"
	"net"
	"strings"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	cnicurrent "github.com/containernetworking/cni/pkg/types/current"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/pkg/log"
	"k8s.io/kubernetes/pkg/kubelet/dockershim/network/hostport"
)

// networkStart sets up the sandbox's network and returns the pod IP on success
// or an error
func (s *Server) networkStart(ctx context.Context, sb *sandbox.Sandbox) (podIPs []string, result cnitypes.Result, err error) {
	if sb.HostNetwork() {
		return s.hostIPs, nil, nil
	}

	podNetwork, err := s.newPodNetwork(sb)
	if err != nil {
		return
	}

	// Ensure network resources are cleaned up if the plugin succeeded
	// but an error happened between plugin success and the end of networkStart()
	defer func() {
		if err != nil {
			s.networkStop(ctx, sb)
		}
	}()

	_, err = s.netPlugin.SetUpPod(podNetwork)
	if err != nil {
		err = fmt.Errorf("failed to create pod network sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
		return
	}

	tmp, err := s.netPlugin.GetPodNetworkStatus(podNetwork)
	if err != nil {
		err = fmt.Errorf("failed to get network status for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
		return
	}

	// only one cnitypes.Result is returned since newPodNetwork sets Networks list empty
	result = tmp[0]
	log.Debugf(ctx, "CNI setup result: %v", result)

	network, err := cnicurrent.GetResult(result)
	if err != nil {
		err = fmt.Errorf("failed to get network JSON for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
		return
	}

	for idx, podIPConfig := range network.IPs {
		podIP := strings.Split(podIPConfig.Address.String(), "/")[0]

		// Apply the hostport mappings only for the first IP to avoid allocating
		// the same host port twice
		if idx == 0 && len(sb.PortMappings()) > 0 {
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

		podIPs = append(podIPs, podIP)
	}

	log.Debugf(ctx, "found POD IPs: %v", podIPs)
	return podIPs, result, err
}

// getSandboxIP retrieves the IP address for the sandbox
func (s *Server) getSandboxIPs(sb *sandbox.Sandbox) (podIPs []string, err error) {
	if sb.HostNetwork() {
		return s.hostIPs, nil
	}

	podNetwork, err := s.newPodNetwork(sb)
	if err != nil {
		return nil, err
	}
	result, err := s.netPlugin.GetPodNetworkStatus(podNetwork)
	if err != nil {
		return nil, fmt.Errorf("failed to get network status for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
	}

	res, err := cnicurrent.GetResult(result[0])
	if err != nil {
		return nil, fmt.Errorf("failed to get network JSON for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)
	}

	for _, podIPConfig := range res.IPs {
		podIPs = append(podIPs, strings.Split(podIPConfig.Address.String(), "/")[0])
	}

	return podIPs, nil
}

// networkStop cleans up and removes a pod's network.  It is best-effort and
// must call the network plugin even if the network namespace is already gone
func (s *Server) networkStop(ctx context.Context, sb *sandbox.Sandbox) {
	if sb.HostNetwork() {
		return
	}

	if err := s.hostportManager.Remove(sb.ID(), &hostport.PodPortMapping{
		Name:         sb.Name(),
		PortMappings: sb.PortMappings(),
		HostNetwork:  false,
	}); err != nil {
		log.Warnf(ctx, "failed to remove hostport for pod sandbox %s(%s): %v",
			sb.Name(), sb.ID(), err)
	}

	podNetwork, err := s.newPodNetwork(sb)
	if err != nil {
		log.Warnf(ctx, err.Error())
		return
	}
	if err := s.netPlugin.TearDownPod(podNetwork); err != nil {
		log.Warnf(ctx, "failed to destroy network for pod sandbox %s(%s): %v",
			sb.Name(), sb.ID(), err)
	}
}
