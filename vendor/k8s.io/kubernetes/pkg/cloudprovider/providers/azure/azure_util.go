/*
Copyright 2016 The Kubernetes Authors.

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

package azure

import (
	"fmt"
	"strings"

	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/cloudprovider"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"k8s.io/apimachinery/pkg/types"
)

const (
	loadBalancerMinimumPriority = 500
	loadBalancerMaximumPriority = 4096

	machineIDTemplate           = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s"
	availabilitySetIDTemplate   = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/availabilitySets/%s"
	frontendIPConfigIDTemplate  = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s"
	backendPoolIDTemplate       = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s"
	loadBalancerRuleIDTemplate  = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/loadBalancingRules/%s"
	loadBalancerProbeIDTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/probes/%s"
	securityRuleIDTemplate      = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkSecurityGroups/%s/securityRules/%s"
)

// returns the full identifier of a machine
func (az *Cloud) getMachineID(machineName string) string {
	return fmt.Sprintf(
		machineIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		machineName)
}

// returns the full identifier of an availabilitySet
func (az *Cloud) getAvailabilitySetID(availabilitySetName string) string {
	return fmt.Sprintf(
		availabilitySetIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		availabilitySetName)
}

// returns the full identifier of a loadbalancer frontendipconfiguration.
func (az *Cloud) getFrontendIPConfigID(lbName, backendPoolName string) string {
	return fmt.Sprintf(
		frontendIPConfigIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		lbName,
		backendPoolName)
}

// returns the full identifier of a loadbalancer backendpool.
func (az *Cloud) getBackendPoolID(lbName, backendPoolName string) string {
	return fmt.Sprintf(
		backendPoolIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		lbName,
		backendPoolName)
}

// returns the full identifier of a loadbalancer rule.
func (az *Cloud) getLoadBalancerRuleID(lbName, lbRuleName string) string {
	return fmt.Sprintf(
		loadBalancerRuleIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		lbName,
		lbRuleName)
}

// returns the full identifier of a loadbalancer probe.
func (az *Cloud) getLoadBalancerProbeID(lbName, lbRuleName string) string {
	return fmt.Sprintf(
		loadBalancerProbeIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		lbName,
		lbRuleName)
}

// returns the full identifier of a network security group security rule.
func (az *Cloud) getSecurityRuleID(securityRuleName string) string {
	return fmt.Sprintf(
		securityRuleIDTemplate,
		az.SubscriptionID,
		az.ResourceGroup,
		az.SecurityGroupName,
		securityRuleName)
}

// returns the deepest child's identifier from a full identifier string.
func getLastSegment(ID string) (string, error) {
	parts := strings.Split(ID, "/")
	name := parts[len(parts)-1]
	if len(name) == 0 {
		return "", fmt.Errorf("resource name was missing from identifier")
	}

	return name, nil
}

// returns the equivalent LoadBalancerRule, SecurityRule and LoadBalancerProbe
// protocol types for the given Kubernetes protocol type.
func getProtocolsFromKubernetesProtocol(protocol v1.Protocol) (*network.TransportProtocol, *network.SecurityRuleProtocol, *network.ProbeProtocol, error) {
	var transportProto network.TransportProtocol
	var securityProto network.SecurityRuleProtocol
	var probeProto network.ProbeProtocol

	switch protocol {
	case v1.ProtocolTCP:
		transportProto = network.TransportProtocolTCP
		securityProto = network.TCP
		probeProto = network.ProbeProtocolTCP
		return &transportProto, &securityProto, &probeProto, nil
	case v1.ProtocolUDP:
		transportProto = network.TransportProtocolUDP
		securityProto = network.UDP
		return &transportProto, &securityProto, nil, nil
	default:
		return &transportProto, &securityProto, &probeProto, fmt.Errorf("Only TCP and UDP are supported for Azure LoadBalancers")
	}

}

// This returns the full identifier of the primary NIC for the given VM.
func getPrimaryInterfaceID(machine compute.VirtualMachine) (string, error) {
	if len(*machine.NetworkProfile.NetworkInterfaces) == 1 {
		return *(*machine.NetworkProfile.NetworkInterfaces)[0].ID, nil
	}

	for _, ref := range *machine.NetworkProfile.NetworkInterfaces {
		if *ref.Primary {
			return *ref.ID, nil
		}
	}

	return "", fmt.Errorf("failed to find a primary nic for the vm. vmname=%q", *machine.Name)
}

func getPrimaryIPConfig(nic network.Interface) (*network.InterfaceIPConfiguration, error) {
	if len(*nic.IPConfigurations) == 1 {
		return &((*nic.IPConfigurations)[0]), nil
	}

	for _, ref := range *nic.IPConfigurations {
		if *ref.Primary {
			return &ref, nil
		}
	}

	return nil, fmt.Errorf("failed to determine the determine primary ipconfig. nicname=%q", *nic.Name)
}

// For a load balancer, all frontend ip should reference either a subnet or publicIpAddress.
// Thus Azure do not allow mixed type (public and internal) load balancer.
// So we'd have a separate name for internal load balancer.
func getLoadBalancerName(clusterName string, isInternal bool) string {
	if isInternal {
		return fmt.Sprintf("%s-internal", clusterName)
	}

	return clusterName
}

func getBackendPoolName(clusterName string) string {
	return clusterName
}

func getLoadBalancerRuleName(service *v1.Service, port v1.ServicePort) string {
	return fmt.Sprintf("%s-%s-%d", getRulePrefix(service), port.Protocol, port.Port)
}

func getSecurityRuleName(service *v1.Service, port v1.ServicePort, sourceAddrPrefix string) string {
	safePrefix := strings.Replace(sourceAddrPrefix, "/", "_", -1)
	return fmt.Sprintf("%s-%s-%d-%s", getRulePrefix(service), port.Protocol, port.Port, safePrefix)
}

// This returns a human-readable version of the Service used to tag some resources.
// This is only used for human-readable convenience, and not to filter.
func getServiceName(service *v1.Service) string {
	return fmt.Sprintf("%s/%s", service.Namespace, service.Name)
}

// This returns a prefix for loadbalancer/security rules.
func getRulePrefix(service *v1.Service) string {
	return cloudprovider.GetLoadBalancerName(service)
}

func serviceOwnsRule(service *v1.Service, rule string) bool {
	prefix := getRulePrefix(service)
	return strings.HasPrefix(strings.ToUpper(rule), strings.ToUpper(prefix))
}

func getFrontendIPConfigName(service *v1.Service) string {
	return cloudprovider.GetLoadBalancerName(service)
}

// This returns the next available rule priority level for a given set of security rules.
func getNextAvailablePriority(rules []network.SecurityRule) (int32, error) {
	var smallest int32 = loadBalancerMinimumPriority
	var spread int32 = 1

outer:
	for smallest < loadBalancerMaximumPriority {
		for _, rule := range rules {
			if *rule.Priority == smallest {
				smallest += spread
				continue outer
			}
		}
		// no one else had it
		return smallest, nil
	}

	return -1, fmt.Errorf("SecurityGroup priorities are exhausted")
}

func (az *Cloud) getIPForMachine(nodeName types.NodeName) (string, error) {
	machine, exists, err := az.getVirtualMachine(nodeName)
	if !exists {
		return "", cloudprovider.InstanceNotFound
	}
	if err != nil {
		return "", err
	}

	nicID, err := getPrimaryInterfaceID(machine)
	if err != nil {
		return "", err
	}

	nicName, err := getLastSegment(nicID)
	if err != nil {
		return "", err
	}

	nic, err := az.InterfacesClient.Get(az.ResourceGroup, nicName, "")
	if err != nil {
		return "", err
	}

	ipConfig, err := getPrimaryIPConfig(nic)
	if err != nil {
		return "", err
	}

	targetIP := *ipConfig.PrivateIPAddress
	return targetIP, nil
}
