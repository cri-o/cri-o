package server

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	cnitypes "github.com/containernetworking/cni/pkg/types"
	cnicurrent "github.com/containernetworking/cni/pkg/types/100"
	"github.com/cri-o/ocicni/pkg/ocicni"
	"k8s.io/apimachinery/pkg/api/resource"
	utilnet "k8s.io/utils/net"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/metrics"
)

const (
	cacheDir = "/var/lib/cni/results"
)

// networkStart sets up the sandbox's network and returns the pod IP on success
// or an error.
func (s *Server) networkStart(ctx context.Context, sb *sandbox.Sandbox) (podIPs []string, result cnitypes.Result, retErr error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	overallStart := time.Now()
	// Give a network Start call a full 5 minutes, independent of the context of the request.
	// This is to prevent the CNI plugin from taking an unbounded amount of time,
	// but to still allow a long-running sandbox creation to be cached and reused,
	// rather than failing and recreating it.
	// Adding on top of the specified deadline ensures this deadline will be respected, regardless of
	// how Kubelet's runtime-request-timeout changes.
	startTimeout := 5 * time.Minute
	if initialDeadline, ok := ctx.Deadline(); ok {
		startTimeout += time.Until(initialDeadline)
	}

	startCtx, startCancel := context.WithTimeout(context.Background(), startTimeout)
	defer startCancel()

	if sb.HostNetwork() {
		return nil, nil, nil
	}

	podNetwork, err := s.newPodNetwork(ctx, sb)
	if err != nil {
		return nil, nil, err
	}

	// Ensure network resources are cleaned up if the plugin succeeded
	// but an error happened between plugin success and the end of networkStart()
	defer func() {
		if retErr != nil {
			log.Infof(ctx, "NetworkStart: stopping network for sandbox %s", sb.ID())
			// use a new context to prevent an expired context from preventing a stop
			if err2 := s.networkStop(context.Background(), sb); err2 != nil {
				log.Errorf(ctx, "Error stopping network on cleanup: %v", err2)
			}
		}
	}()

	podSetUpStart := time.Now()

	_, err = s.config.CNIPlugin().SetUpPodWithContext(startCtx, podNetwork)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create pod network sandbox %s(%s): %w", sb.Name(), sb.ID(), err)
	}
	// metric about the CNI network setup operation
	metrics.Instance().MetricOperationsLatencySet("network_setup_pod", podSetUpStart)

	podNetworkStatus, err := s.config.CNIPlugin().GetPodNetworkStatusWithContext(startCtx, podNetwork)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get network status for pod sandbox %s(%s): %w", sb.Name(), sb.ID(), err)
	}

	// only one cnitypes.Result is returned since newPodNetwork sets Networks list empty
	result = podNetworkStatus[0].Result
	log.Debugf(ctx, "CNI setup result: %v", result)

	network, err := cnicurrent.GetResult(result)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get network JSON for pod sandbox %s(%s): %w", sb.Name(), sb.ID(), err)
	}

	// only do portmapping to the first IP of each IP family
	foundIPv4 := false
	foundIPv6 := false
	// cache the portmapping info
	sbID := sb.ID()
	sbName := sb.Name()
	sbPortMappings := sb.PortMappings()
	// iterate over each IP and add the portmap if needed
	for _, podIPConfig := range network.IPs {
		ip := podIPConfig.Address.IP
		podIPs = append(podIPs, ip.String())

		// the pod has host-ports defined
		if len(sbPortMappings) > 0 {
			//nolint:gocritic // using a switch statement is not much different
			if utilnet.IsIPv6(ip) {
				if foundIPv6 {
					// we have already done the portmap for IPv6
					continue
				}
				// found a new IPv6 address, do the portmap
				foundIPv6 = true
			} else if foundIPv4 {
				// we have already done the portmap for IPv4
				continue
			} else {
				// found a new IPv4 address, do the portmap
				foundIPv4 = true
			}

			err = s.hostportManager.Add(sbID, sbName, ip.String(), sbPortMappings)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to add hostport mapping for sandbox %s(%s): %w", sb.Name(), sb.ID(), err)
			}
		}
	}

	log.Debugf(ctx, "Found POD IPs: %v", podIPs)

	// metric about the whole network setup operation
	metrics.Instance().MetricOperationsLatencySet("network_setup_overall", overallStart)

	return podIPs, result, err
}

// getSandboxIP retrieves the IP address for the sandbox.
func (s *Server) getSandboxIPs(ctx context.Context, sb *sandbox.Sandbox) ([]string, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	if sb.HostNetwork() {
		return nil, nil
	}

	podNetwork, err := s.newPodNetwork(ctx, sb)
	if err != nil {
		return nil, err
	}

	podNetworkStatus, err := s.config.CNIPlugin().GetPodNetworkStatus(podNetwork)
	if err != nil {
		return nil, fmt.Errorf("failed to get network status for pod sandbox %s(%s): %w", sb.Name(), sb.ID(), err)
	}

	res, err := cnicurrent.GetResult(podNetworkStatus[0].Result)
	if err != nil {
		return nil, fmt.Errorf("failed to get network JSON for pod sandbox %s(%s): %w", sb.Name(), sb.ID(), err)
	}

	podIPs := make([]string, 0, len(res.IPs))
	for _, podIPConfig := range res.IPs {
		podIPs = append(podIPs, podIPConfig.Address.IP.String())
	}

	return podIPs, nil
}

// networkStop cleans up and removes a pod's network.  It is best-effort and
// must call the network plugin even if the network namespace is already gone.
func (s *Server) networkStop(ctx context.Context, sb *sandbox.Sandbox) error {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	if sb.HostNetwork() || sb.NetworkStopped() {
		return nil
	}
	// give a network stop call 1 minutes, half of a StopPod request timeout limit
	stopCtx, stopCancel := context.WithTimeout(ctx, 1*time.Minute)
	defer stopCancel()

	// portMapping removal does not need the IP address
	if err := s.hostportManager.Remove(sb.ID(), sb.PortMappings()); err != nil {
		log.Warnf(ctx, "Failed to remove hostport for pod sandbox %s(%s): %v",
			sb.Name(), sb.ID(), err)
	}

	podNetwork, err := s.newPodNetwork(ctx, sb)
	if err != nil {
		return fmt.Errorf("failed to create pod network for sandbox %s(%s): %w", sb.Name(), sb.ID(), err)
	}

	// Check if the network namespace file exists and is valid before attempting CNI teardown.
	// If the file doesn't exist or is invalid, we should still attempt CNI teardown using cached information
	// to prevent IP leaks, but we'll mark the network as stopped regardless of the outcome.
	netnsValid := true

	if podNetwork.NetNS == "" {
		// Network namespace path is unexpectedly empty. This can happen when:
		// 1) infra container process died
		// 2) namespace not properly initialized
		// 3) namespace was already cleaned up
		log.Warnf(ctx, "Network namespace path is empty for pod sandbox %s(%s), attempting CNI teardown with cached info",
			sb.Name(), sb.ID())

		netnsValid = false
	} else {
		if _, statErr := os.Stat(podNetwork.NetNS); statErr != nil {
			// Network namespace file doesn't exist, but we should still attempt CNI teardown
			log.Debugf(ctx, "Network namespace file %s does not exist for pod sandbox %s(%s), attempting CNI teardown with cached info",
				podNetwork.NetNS, sb.Name(), sb.ID())

			netnsValid = false
		} else if validateErr := s.validateNetworkNamespace(podNetwork.NetNS); validateErr != nil {
			// Network namespace file exists but is invalid (e.g., corrupted or fake file)
			log.Warnf(ctx, "Network namespace file %s is invalid for pod sandbox %s(%s): %v, removing and attempting CNI teardown with cached info",
				podNetwork.NetNS, sb.Name(), sb.ID(), validateErr)
			s.cleanupNetns(ctx, podNetwork.NetNS, sb)

			netnsValid = false
		}
	}

	// Always attempt CNI teardown to prevent IP leaks, even if netns is invalid.
	if err := s.config.CNIPlugin().TearDownPodWithContext(stopCtx, podNetwork); err != nil {
		if !netnsValid {
			// This is expected when the network namespace is missing/invalid.
			log.Debugf(ctx, "CNI teardown failed due to missing/invalid network namespace for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)

			// Clean up CNI result files even when NetNS is invalid.
			s.cleanupCNIResultFiles(ctx, sb.ID())

			return sb.SetNetworkStopped(ctx, true)
		}

		log.Warnf(ctx, "Failed to destroy network for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), err)

		// If the network namespace exists but CNI teardown failed, try to clean it up.
		if podNetwork.NetNS != "" && netnsValid {
			if _, statErr := os.Stat(podNetwork.NetNS); statErr == nil {
				// Clean up the netns file since CNI teardown failed.
				s.cleanupNetns(ctx, podNetwork.NetNS, sb)
			}
		}

		// Clean up CNI result files if CNI teardown failed.
		s.cleanupCNIResultFiles(ctx, sb.ID())

		// Even if CNI teardown failed, mark network as stopped to prevent retry loops.
		if setErr := sb.SetNetworkStopped(ctx, true); setErr != nil {
			log.Warnf(ctx, "Failed to set network stopped for pod sandbox %s(%s): %v", sb.Name(), sb.ID(), setErr)
		}

		return fmt.Errorf("network teardown failed for pod sandbox %s(%s): %w", sb.Name(), sb.ID(), err)
	}

	return sb.SetNetworkStopped(ctx, true)
}

// cleanupCNIResultFiles removes CNI result files for a given container ID.
// This is called when CNI teardown fails to prevent stale result files from accumulating.
func (s *Server) cleanupCNIResultFiles(ctx context.Context, containerID string) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		log.Warnf(ctx, "Failed to read CNI cache directory %s: %v", cacheDir, err)

		return
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.Contains(entry.Name(), containerID) {
			filePath := filepath.Join(cacheDir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				log.Warnf(ctx, "Failed to remove CNI result file %s: %v", filePath, err)
			} else {
				log.Infof(ctx, "Cleaned up CNI result file %s for container %s", entry.Name(), containerID)
			}
		}
	}
}

func (s *Server) newPodNetwork(ctx context.Context, sb *sandbox.Sandbox) (ocicni.PodNetwork, error) {
	_, span := log.StartSpan(ctx)
	defer span.End()

	var egress, ingress int64

	if val, ok := sb.Annotations()["kubernetes.io/egress-bandwidth"]; ok {
		egressQ, err := resource.ParseQuantity(val)
		if err != nil {
			return ocicni.PodNetwork{}, fmt.Errorf("failed to parse egress bandwidth: %w", err)
		} else if iegress, isok := egressQ.AsInt64(); isok {
			egress = iegress
		}
	}

	if val, ok := sb.Annotations()["kubernetes.io/ingress-bandwidth"]; ok {
		ingressQ, err := resource.ParseQuantity(val)
		if err != nil {
			return ocicni.PodNetwork{}, fmt.Errorf("failed to parse ingress bandwidth: %w", err)
		} else if iingress, isok := ingressQ.AsInt64(); isok {
			ingress = iingress
		}
	}

	var bwConfig *ocicni.BandwidthConfig

	if ingress > 0 || egress > 0 {
		bwConfig = &ocicni.BandwidthConfig{}
		if ingress > 0 {
			bwConfig.IngressRate = uint64(ingress)
			bwConfig.IngressBurst = math.MaxUint32*8 - 1 // 4GB burst limit
		}

		if egress > 0 {
			bwConfig.EgressRate = uint64(egress)
			bwConfig.EgressBurst = math.MaxUint32*8 - 1 // 4GB burst limit
		}
	}

	network := s.config.CNIPlugin().GetDefaultNetworkName()
	podAnnotations := sb.Annotations()
	// To address typecheck linter.
	if podAnnotations == nil {
		podAnnotations = make(map[string]string)
	}

	return ocicni.PodNetwork{
		Name:      sb.KubeName(),
		Namespace: sb.Namespace(),
		UID:       sb.Metadata().GetUid(),
		Networks:  []ocicni.NetAttachment{},
		ID:        sb.ID(),
		NetNS:     sb.NetNsPath(),
		RuntimeConfig: map[string]ocicni.RuntimeConfig{
			network: {
				Bandwidth:      bwConfig,
				CgroupPath:     sb.CgroupParent(),
				PodAnnotations: &podAnnotations,
			},
		},
	}, nil
}

// networkGC cleans up any resources concerned with stale pods (pods not
// included in validPods).
func (s *Server) networkGC(ctx context.Context, validPods []*sandbox.Sandbox) error {
	_, span := log.StartSpan(ctx)
	defer span.End()

	return s.config.CNIPluginGC(ctx, func() ([]*ocicni.PodNetwork, error) {
		validPodNetworks := make([]*ocicni.PodNetwork, len(validPods))

		for i := range validPods {
			podNetwork, err := s.newPodNetwork(ctx, validPods[i])
			if err != nil {
				return nil, err
			}

			validPodNetworks[i] = &podNetwork
		}

		return validPodNetworks, nil
	})
}

// WaitForCNIPlugin waits for the CNI plugin to be ready.
func (s *Server) waitForCNIPlugin(ctx context.Context, sboxName string) error {
	if err := s.config.CNIPluginReadyOrError(); err != nil {
		watcher := s.config.CNIPluginAddWatcher()

		log.Infof(ctx, "CNI plugin not ready. Waiting to create %s", sboxName)

		if ready := <-watcher; !ready {
			return fmt.Errorf("server shutdown before CNI plugin was ready: %w", err)
		}

		log.Infof(ctx, "CNI plugin is now ready. Continuing to create %s", sboxName)
	}

	return nil
}
