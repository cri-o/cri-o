/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package hostport

import (
	"net"

	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	v1 "k8s.io/api/core/v1"
	utilnet "k8s.io/utils/net"
)

// HostPortManager is an interface for adding and removing hostport for a given pod sandbox.
//
//nolint:golint // no reason to change the type name now "type name will be used as hostport.HostPortManager by other packages"
type HostPortManager interface {
	// Add implements port mappings.
	// id should be a unique identifier for a pod, e.g. podSandboxID.
	// podPortMapping is the associated port mapping information for the pod.
	// natInterfaceName is the interface that localhost uses to talk to the given pod, if known.
	Add(id string, podPortMapping *PodPortMapping, natInterfaceName string) error
	// Remove cleans up matching port mappings
	// Remove must be able to clean up port mappings without pod IP
	Remove(id string, podPortMapping *PodPortMapping) error
}

// NewHostportManager creates a new HostPortManager for this system
func NewHostportManager() HostPortManager {
	hm, err := newHostportManagerNFTables()
	if err != nil {
		logrus.Infof("Could not create nftables-based HostPortManager (%v); falling back to iptables", err)
		hm = newHostportManagerIPTables()
	}
	return hm
}

// PortMapping represents a network port in a container.
type PortMapping struct {
	HostPort      int32
	ContainerPort int32
	Protocol      v1.Protocol
	HostIP        string
}

// PodPortMapping represents a pod's network state and associated container port mappings.
type PodPortMapping struct {
	Namespace    string
	Name         string
	PortMappings []*PortMapping
	IP           net.IP
}

// gatherHostportMappings returns all the PortMappings which has hostport for a pod
// it filters the PortMappings that use HostIP and doesn't match the IP family specified.
func gatherHostportMappings(podPortMapping *PodPortMapping, isIPv6 bool) []*PortMapping {
	mappings := []*PortMapping{}
	for _, pm := range podPortMapping.PortMappings {
		if pm.HostPort <= 0 {
			continue
		}
		if pm.HostIP != "" && utilnet.IsIPv6String(pm.HostIP) != isIPv6 {
			continue
		}
		mappings = append(mappings, pm)
	}
	return mappings
}

func getPodFullName(pod *PodPortMapping) string {
	// Use underscore as the delimiter because it is not allowed in pod name
	// (DNS subdomain format), while allowed in the container name format.
	return pod.Name + "_" + pod.Namespace
}

func getNetlinkFamily(isIPv6 bool) netlink.InetFamily {
	if isIPv6 {
		return unix.AF_INET6
	}
	return unix.AF_INET
}
