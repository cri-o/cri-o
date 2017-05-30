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
	"strconv"
	"strings"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/kubernetes/pkg/api/v1"
	serviceapi "k8s.io/kubernetes/pkg/api/v1/service"
	"k8s.io/kubernetes/pkg/cloudprovider"

	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/types"
)

// ServiceAnnotationLoadBalancerInternal is the annotation used on the service
const ServiceAnnotationLoadBalancerInternal = "service.beta.kubernetes.io/azure-load-balancer-internal"

// GetLoadBalancer returns whether the specified load balancer exists, and
// if so, what its status is.
func (az *Cloud) GetLoadBalancer(clusterName string, service *v1.Service) (status *v1.LoadBalancerStatus, exists bool, err error) {
	isInternal := requiresInternalLoadBalancer(service)
	lbName := getLoadBalancerName(clusterName, isInternal)
	serviceName := getServiceName(service)

	lb, existsLb, err := az.getAzureLoadBalancer(lbName)
	if err != nil {
		return nil, false, err
	}
	if !existsLb {
		glog.V(5).Infof("get(%s): lb(%s) - doesn't exist", serviceName, lbName)
		return nil, false, nil
	}

	var lbIP *string

	if isInternal {
		lbFrontendIPConfigName := getFrontendIPConfigName(service)
		for _, ipConfiguration := range *lb.FrontendIPConfigurations {
			if lbFrontendIPConfigName == *ipConfiguration.Name {
				lbIP = ipConfiguration.PrivateIPAddress
				break
			}
		}
	} else {
		// TODO: Consider also read address from lb's FrontendIPConfigurations
		pipName, err := az.getPublicIPName(clusterName, service)
		if err != nil {
			return nil, false, err
		}
		pip, existsPip, err := az.getPublicIPAddress(pipName)
		if err != nil {
			return nil, false, err
		}
		if existsPip {
			lbIP = pip.IPAddress
		}
	}

	if lbIP == nil {
		glog.V(5).Infof("get(%s): lb(%s) - IP doesn't exist", serviceName, lbName)
		return nil, false, nil
	}

	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{{IP: *lbIP}},
	}, true, nil
}

func (az *Cloud) getPublicIPName(clusterName string, service *v1.Service) (string, error) {
	loadBalancerIP := service.Spec.LoadBalancerIP
	if len(loadBalancerIP) == 0 {
		return fmt.Sprintf("%s-%s", clusterName, cloudprovider.GetLoadBalancerName(service)), nil
	}

	list, err := az.PublicIPAddressesClient.List(az.ResourceGroup)
	if err != nil {
		return "", err
	}

	if list.Value != nil {
		for ix := range *list.Value {
			ip := &(*list.Value)[ix]
			if ip.PublicIPAddressPropertiesFormat.IPAddress != nil &&
				*ip.PublicIPAddressPropertiesFormat.IPAddress == loadBalancerIP {
				return *ip.Name, nil
			}
		}
	}
	// TODO: follow next link here? Will there really ever be that many public IPs?

	return "", fmt.Errorf("user supplied IP Address %s was not found", loadBalancerIP)
}

// EnsureLoadBalancer creates a new load balancer 'name', or updates the existing one. Returns the status of the balancer
func (az *Cloud) EnsureLoadBalancer(clusterName string, service *v1.Service, nodes []*v1.Node) (*v1.LoadBalancerStatus, error) {
	isInternal := requiresInternalLoadBalancer(service)
	lbName := getLoadBalancerName(clusterName, isInternal)

	// When a client updates the internal load balancer annotation,
	// the service may be switched from an internal LB to a public one, or vise versa.
	// Here we'll firstly ensure service do not lie in the opposite LB.
	err := az.cleanupLoadBalancer(clusterName, service, !isInternal)
	if err != nil {
		return nil, err
	}

	// Also clean up public ip resource, since service might be switched from public load balancer type.
	if isInternal {
		err = az.cleanupPublicIP(clusterName, service)
		if err != nil {
			return nil, err
		}
	}

	serviceName := getServiceName(service)
	glog.V(5).Infof("ensure(%s): START clusterName=%q lbName=%q", serviceName, clusterName, lbName)

	sg, err := az.SecurityGroupsClient.Get(az.ResourceGroup, az.SecurityGroupName, "")
	if err != nil {
		return nil, err
	}
	sg, sgNeedsUpdate, err := az.reconcileSecurityGroup(sg, clusterName, service, true /* wantLb */)
	if err != nil {
		return nil, err
	}
	if sgNeedsUpdate {
		glog.V(3).Infof("ensure(%s): sg(%s) - updating", serviceName, *sg.Name)
		// azure-sdk-for-go introduced contraint validation which breaks the updating here if we don't set these
		// to nil. This is a workaround until https://github.com/Azure/go-autorest/issues/112 is fixed
		sg.SecurityGroupPropertiesFormat.NetworkInterfaces = nil
		sg.SecurityGroupPropertiesFormat.Subnets = nil
		_, err := az.SecurityGroupsClient.CreateOrUpdate(az.ResourceGroup, *sg.Name, sg, nil)
		if err != nil {
			return nil, err
		}
	}

	lb, existsLb, err := az.getAzureLoadBalancer(lbName)
	if err != nil {
		return nil, err
	}
	if !existsLb {
		lb = network.LoadBalancer{
			Name:                         &lbName,
			Location:                     &az.Location,
			LoadBalancerPropertiesFormat: &network.LoadBalancerPropertiesFormat{},
		}
	}

	var lbIP *string
	var fipConfigurationProperties *network.FrontendIPConfigurationPropertiesFormat

	if isInternal {
		subnet, existsSubnet, err := az.getSubnet(az.VnetName, az.SubnetName)
		if err != nil {
			return nil, err
		}

		if !existsSubnet {
			return nil, fmt.Errorf("ensure(%s): lb(%s) - failed to get subnet: %s/%s", serviceName, lbName, az.VnetName, az.SubnetName)
		}

		configProperties := network.FrontendIPConfigurationPropertiesFormat{
			Subnet: &network.Subnet{
				ID: subnet.ID,
			},
		}

		loadBalancerIP := service.Spec.LoadBalancerIP
		if loadBalancerIP != "" {
			configProperties.PrivateIPAllocationMethod = network.Static
			configProperties.PrivateIPAddress = &loadBalancerIP
			lbIP = &loadBalancerIP
		} else {
			// We'll need to call GetLoadBalancer later to retrieve allocated IP.
			configProperties.PrivateIPAllocationMethod = network.Dynamic
		}

		fipConfigurationProperties = &configProperties
	} else {
		pipName, err := az.getPublicIPName(clusterName, service)
		if err != nil {
			return nil, err
		}
		pip, err := az.ensurePublicIPExists(serviceName, pipName)
		if err != nil {
			return nil, err
		}

		lbIP = pip.IPAddress
		fipConfigurationProperties = &network.FrontendIPConfigurationPropertiesFormat{
			PublicIPAddress: &network.PublicIPAddress{ID: pip.ID},
		}
	}

	lb, lbNeedsUpdate, err := az.reconcileLoadBalancer(lb, fipConfigurationProperties, clusterName, service, nodes)
	if err != nil {
		return nil, err
	}
	if !existsLb || lbNeedsUpdate {
		glog.V(3).Infof("ensure(%s): lb(%s) - updating", serviceName, lbName)
		_, err = az.LoadBalancerClient.CreateOrUpdate(az.ResourceGroup, *lb.Name, lb, nil)
		if err != nil {
			return nil, err
		}
	}

	// Add the machines to the backend pool if they're not already
	lbBackendName := getBackendPoolName(clusterName)
	lbBackendPoolID := az.getBackendPoolID(lbName, lbBackendName)
	hostUpdates := make([]func() error, len(nodes))
	for i, node := range nodes {
		localNodeName := node.Name
		f := func() error {
			err := az.ensureHostInPool(serviceName, types.NodeName(localNodeName), lbBackendPoolID)
			if err != nil {
				return fmt.Errorf("ensure(%s): lb(%s) - failed to ensure host in pool: %q", serviceName, lbName, err)
			}
			return nil
		}
		hostUpdates[i] = f
	}

	errs := utilerrors.AggregateGoroutines(hostUpdates...)
	if errs != nil {
		return nil, utilerrors.Flatten(errs)
	}

	glog.V(2).Infof("ensure(%s): lb(%s) finished", serviceName, lbName)

	if lbIP == nil {
		lbStatus, exists, err := az.GetLoadBalancer(clusterName, service)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("ensure(%s): lb(%s) - failed to get back load balancer", serviceName, lbName)
		}
		return lbStatus, nil
	}

	return &v1.LoadBalancerStatus{
		Ingress: []v1.LoadBalancerIngress{{IP: *lbIP}},
	}, nil
}

// UpdateLoadBalancer updates hosts under the specified load balancer.
func (az *Cloud) UpdateLoadBalancer(clusterName string, service *v1.Service, nodes []*v1.Node) error {
	_, err := az.EnsureLoadBalancer(clusterName, service, nodes)
	return err
}

// EnsureLoadBalancerDeleted deletes the specified load balancer if it
// exists, returning nil if the load balancer specified either didn't exist or
// was successfully deleted.
// This construction is useful because many cloud providers' load balancers
// have multiple underlying components, meaning a Get could say that the LB
// doesn't exist even if some part of it is still laying around.
func (az *Cloud) EnsureLoadBalancerDeleted(clusterName string, service *v1.Service) error {
	isInternal := requiresInternalLoadBalancer(service)
	lbName := getLoadBalancerName(clusterName, isInternal)
	serviceName := getServiceName(service)

	glog.V(5).Infof("delete(%s): START clusterName=%q lbName=%q", serviceName, clusterName, lbName)

	err := az.cleanupLoadBalancer(clusterName, service, isInternal)
	if err != nil {
		return err
	}

	if !isInternal {
		err = az.cleanupPublicIP(clusterName, service)
		if err != nil {
			return err
		}
	}

	sg, existsSg, err := az.getSecurityGroup()
	if err != nil {
		return err
	}
	if existsSg {
		reconciledSg, sgNeedsUpdate, reconcileErr := az.reconcileSecurityGroup(sg, clusterName, service, false /* wantLb */)
		if reconcileErr != nil {
			return reconcileErr
		}
		if sgNeedsUpdate {
			glog.V(3).Infof("delete(%s): sg(%s) - updating", serviceName, az.SecurityGroupName)
			// azure-sdk-for-go introduced contraint validation which breaks the updating here if we don't set these
			// to nil. This is a workaround until https://github.com/Azure/go-autorest/issues/112 is fixed
			sg.SecurityGroupPropertiesFormat.NetworkInterfaces = nil
			sg.SecurityGroupPropertiesFormat.Subnets = nil
			_, err := az.SecurityGroupsClient.CreateOrUpdate(az.ResourceGroup, *reconciledSg.Name, reconciledSg, nil)
			if err != nil {
				return err
			}
		}
	}

	glog.V(2).Infof("delete(%s): FINISH", serviceName)
	return nil
}

func (az *Cloud) cleanupLoadBalancer(clusterName string, service *v1.Service, isInternalLb bool) error {
	lbName := getLoadBalancerName(clusterName, isInternalLb)
	serviceName := getServiceName(service)

	glog.V(10).Infof("ensure lb deleted: clusterName=%q, serviceName=%s, lbName=%q", clusterName, serviceName, lbName)

	lb, existsLb, err := az.getAzureLoadBalancer(lbName)
	if err != nil {
		return err
	}
	if existsLb {
		lb, lbNeedsUpdate, reconcileErr := az.reconcileLoadBalancer(lb, nil, clusterName, service, []*v1.Node{})
		if reconcileErr != nil {
			return reconcileErr
		}
		if lbNeedsUpdate {
			if len(*lb.FrontendIPConfigurations) > 0 {
				glog.V(3).Infof("delete(%s): lb(%s) - updating", serviceName, lbName)
				_, err = az.LoadBalancerClient.CreateOrUpdate(az.ResourceGroup, *lb.Name, lb, nil)
				if err != nil {
					return err
				}
			} else {
				glog.V(3).Infof("delete(%s): lb(%s) - deleting; no remaining frontendipconfigs", serviceName, lbName)

				_, err = az.LoadBalancerClient.Delete(az.ResourceGroup, lbName, nil)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (az *Cloud) cleanupPublicIP(clusterName string, service *v1.Service) error {
	serviceName := getServiceName(service)

	// Only delete an IP address if we created it.
	if service.Spec.LoadBalancerIP == "" {
		pipName, err := az.getPublicIPName(clusterName, service)
		if err != nil {
			return err
		}
		err = az.ensurePublicIPDeleted(serviceName, pipName)
		if err != nil {
			return err
		}
	}

	return nil
}

func (az *Cloud) ensurePublicIPExists(serviceName, pipName string) (*network.PublicIPAddress, error) {
	pip, existsPip, err := az.getPublicIPAddress(pipName)
	if err != nil {
		return nil, err
	}
	if existsPip {
		return &pip, nil
	}

	pip.Name = to.StringPtr(pipName)
	pip.Location = to.StringPtr(az.Location)
	pip.PublicIPAddressPropertiesFormat = &network.PublicIPAddressPropertiesFormat{
		PublicIPAllocationMethod: network.Static,
	}
	pip.Tags = &map[string]*string{"service": &serviceName}

	glog.V(3).Infof("ensure(%s): pip(%s) - creating", serviceName, *pip.Name)
	_, err = az.PublicIPAddressesClient.CreateOrUpdate(az.ResourceGroup, *pip.Name, pip, nil)
	if err != nil {
		return nil, err
	}

	pip, err = az.PublicIPAddressesClient.Get(az.ResourceGroup, *pip.Name, "")
	if err != nil {
		return nil, err
	}

	return &pip, nil

}

func (az *Cloud) ensurePublicIPDeleted(serviceName, pipName string) error {
	_, deleteErr := az.PublicIPAddressesClient.Delete(az.ResourceGroup, pipName, nil)
	_, realErr := checkResourceExistsFromError(deleteErr)
	if realErr != nil {
		return nil
	}
	return nil
}

// This ensures load balancer exists and the frontend ip config is setup.
// This also reconciles the Service's Ports  with the LoadBalancer config.
// This entails adding rules/probes for expected Ports and removing stale rules/ports.
func (az *Cloud) reconcileLoadBalancer(lb network.LoadBalancer, fipConfigurationProperties *network.FrontendIPConfigurationPropertiesFormat, clusterName string, service *v1.Service, nodes []*v1.Node) (network.LoadBalancer, bool, error) {
	isInternal := requiresInternalLoadBalancer(service)
	lbName := getLoadBalancerName(clusterName, isInternal)
	serviceName := getServiceName(service)
	lbFrontendIPConfigName := getFrontendIPConfigName(service)
	lbFrontendIPConfigID := az.getFrontendIPConfigID(lbName, lbFrontendIPConfigName)
	lbBackendPoolName := getBackendPoolName(clusterName)
	lbBackendPoolID := az.getBackendPoolID(lbName, lbBackendPoolName)

	wantLb := fipConfigurationProperties != nil
	dirtyLb := false

	// Ensure LoadBalancer's Backend Pool Configuration
	if wantLb {
		newBackendPools := []network.BackendAddressPool{}
		if lb.BackendAddressPools != nil {
			newBackendPools = *lb.BackendAddressPools
		}

		foundBackendPool := false
		for _, bp := range newBackendPools {
			if strings.EqualFold(*bp.Name, lbBackendPoolName) {
				glog.V(10).Infof("reconcile(%s)(%t): lb backendpool - found wanted backendpool. not adding anything", serviceName, wantLb)
				foundBackendPool = true
				break
			} else {
				glog.V(10).Infof("reconcile(%s)(%t): lb backendpool - found other backendpool %s", serviceName, wantLb, *bp.Name)
			}
		}
		if !foundBackendPool {
			newBackendPools = append(newBackendPools, network.BackendAddressPool{
				Name: to.StringPtr(lbBackendPoolName),
			})
			glog.V(10).Infof("reconcile(%s)(%t): lb backendpool - adding backendpool", serviceName, wantLb)

			dirtyLb = true
			lb.BackendAddressPools = &newBackendPools
		}
	}

	// Ensure LoadBalancer's Frontend IP Configurations
	dirtyConfigs := false
	newConfigs := []network.FrontendIPConfiguration{}
	if lb.FrontendIPConfigurations != nil {
		newConfigs = *lb.FrontendIPConfigurations
	}
	if !wantLb {
		for i := len(newConfigs) - 1; i >= 0; i-- {
			config := newConfigs[i]
			if strings.EqualFold(*config.Name, lbFrontendIPConfigName) {
				glog.V(3).Infof("reconcile(%s)(%t): lb frontendconfig(%s) - dropping", serviceName, wantLb, lbFrontendIPConfigName)
				newConfigs = append(newConfigs[:i], newConfigs[i+1:]...)
				dirtyConfigs = true
			}
		}
	} else {
		foundConfig := false
		for _, config := range newConfigs {
			if strings.EqualFold(*config.Name, lbFrontendIPConfigName) {
				foundConfig = true
				break
			}
		}
		if !foundConfig {
			newConfigs = append(newConfigs,
				network.FrontendIPConfiguration{
					Name: to.StringPtr(lbFrontendIPConfigName),
					FrontendIPConfigurationPropertiesFormat: fipConfigurationProperties,
				})
			glog.V(10).Infof("reconcile(%s)(%t): lb frontendconfig(%s) - adding", serviceName, wantLb, lbFrontendIPConfigName)
			dirtyConfigs = true
		}
	}
	if dirtyConfigs {
		dirtyLb = true
		lb.FrontendIPConfigurations = &newConfigs
	}

	// update probes/rules
	var ports []v1.ServicePort
	if wantLb {
		ports = service.Spec.Ports
	} else {
		ports = []v1.ServicePort{}
	}

	var expectedProbes []network.Probe
	var expectedRules []network.LoadBalancingRule
	for _, port := range ports {
		lbRuleName := getLoadBalancerRuleName(service, port)

		transportProto, _, probeProto, err := getProtocolsFromKubernetesProtocol(port.Protocol)
		if err != nil {
			return lb, false, err
		}

		if serviceapi.NeedsHealthCheck(service) {
			if port.Protocol == v1.ProtocolUDP {
				// ERROR: this isn't supported
				// health check (aka source ip preservation) is not
				// compatible with UDP (it uses an HTTP check)
				return lb, false, fmt.Errorf("services requiring health checks are incompatible with UDP ports")
			}

			podPresencePath, podPresencePort := serviceapi.GetServiceHealthCheckPathPort(service)

			expectedProbes = append(expectedProbes, network.Probe{
				Name: &lbRuleName,
				ProbePropertiesFormat: &network.ProbePropertiesFormat{
					RequestPath:       to.StringPtr(podPresencePath),
					Protocol:          network.ProbeProtocolHTTP,
					Port:              to.Int32Ptr(podPresencePort),
					IntervalInSeconds: to.Int32Ptr(5),
					NumberOfProbes:    to.Int32Ptr(2),
				},
			})
		} else if port.Protocol != v1.ProtocolUDP {
			// we only add the expected probe if we're doing TCP
			expectedProbes = append(expectedProbes, network.Probe{
				Name: &lbRuleName,
				ProbePropertiesFormat: &network.ProbePropertiesFormat{
					Protocol:          *probeProto,
					Port:              to.Int32Ptr(port.NodePort),
					IntervalInSeconds: to.Int32Ptr(5),
					NumberOfProbes:    to.Int32Ptr(2),
				},
			})
		}

		loadDistribution := network.Default
		if service.Spec.SessionAffinity == v1.ServiceAffinityClientIP {
			loadDistribution = network.SourceIP
		}
		expectedRule := network.LoadBalancingRule{
			Name: &lbRuleName,
			LoadBalancingRulePropertiesFormat: &network.LoadBalancingRulePropertiesFormat{
				Protocol: *transportProto,
				FrontendIPConfiguration: &network.SubResource{
					ID: to.StringPtr(lbFrontendIPConfigID),
				},
				BackendAddressPool: &network.SubResource{
					ID: to.StringPtr(lbBackendPoolID),
				},
				LoadDistribution: loadDistribution,
				FrontendPort:     to.Int32Ptr(port.Port),
				BackendPort:      to.Int32Ptr(port.Port),
				EnableFloatingIP: to.BoolPtr(true),
			},
		}

		// we didn't construct the probe objects for UDP because they're not used/needed/allowed
		if port.Protocol != v1.ProtocolUDP {
			expectedRule.Probe = &network.SubResource{
				ID: to.StringPtr(az.getLoadBalancerProbeID(lbName, lbRuleName)),
			}
		}

		expectedRules = append(expectedRules, expectedRule)
	}

	// remove unwanted probes
	dirtyProbes := false
	var updatedProbes []network.Probe
	if lb.Probes != nil {
		updatedProbes = *lb.Probes
	}
	for i := len(updatedProbes) - 1; i >= 0; i-- {
		existingProbe := updatedProbes[i]
		if serviceOwnsRule(service, *existingProbe.Name) {
			glog.V(10).Infof("reconcile(%s)(%t): lb probe(%s) - considering evicting", serviceName, wantLb, *existingProbe.Name)
			keepProbe := false
			if findProbe(expectedProbes, existingProbe) {
				glog.V(10).Infof("reconcile(%s)(%t): lb probe(%s) - keeping", serviceName, wantLb, *existingProbe.Name)
				keepProbe = true
			}
			if !keepProbe {
				updatedProbes = append(updatedProbes[:i], updatedProbes[i+1:]...)
				glog.V(10).Infof("reconcile(%s)(%t): lb probe(%s) - dropping", serviceName, wantLb, *existingProbe.Name)
				dirtyProbes = true
			}
		}
	}
	// add missing, wanted probes
	for _, expectedProbe := range expectedProbes {
		foundProbe := false
		if findProbe(updatedProbes, expectedProbe) {
			glog.V(10).Infof("reconcile(%s)(%t): lb probe(%s) - already exists", serviceName, wantLb, *expectedProbe.Name)
			foundProbe = true
		}
		if !foundProbe {
			glog.V(10).Infof("reconcile(%s)(%t): lb probe(%s) - adding", serviceName, wantLb, *expectedProbe.Name)
			updatedProbes = append(updatedProbes, expectedProbe)
			dirtyProbes = true
		}
	}
	if dirtyProbes {
		dirtyLb = true
		lb.Probes = &updatedProbes
	}

	// update rules
	dirtyRules := false
	var updatedRules []network.LoadBalancingRule
	if lb.LoadBalancingRules != nil {
		updatedRules = *lb.LoadBalancingRules
	}
	// update rules: remove unwanted
	for i := len(updatedRules) - 1; i >= 0; i-- {
		existingRule := updatedRules[i]
		if serviceOwnsRule(service, *existingRule.Name) {
			keepRule := false
			glog.V(10).Infof("reconcile(%s)(%t): lb rule(%s) - considering evicting", serviceName, wantLb, *existingRule.Name)
			if findRule(expectedRules, existingRule) {
				glog.V(10).Infof("reconcile(%s)(%t): lb rule(%s) - keeping", serviceName, wantLb, *existingRule.Name)
				keepRule = true
			}
			if !keepRule {
				glog.V(3).Infof("reconcile(%s)(%t): lb rule(%s) - dropping", serviceName, wantLb, *existingRule.Name)
				updatedRules = append(updatedRules[:i], updatedRules[i+1:]...)
				dirtyRules = true
			}
		}
	}
	// update rules: add needed
	for _, expectedRule := range expectedRules {
		foundRule := false
		if findRule(updatedRules, expectedRule) {
			glog.V(10).Infof("reconcile(%s)(%t): lb rule(%s) - already exists", serviceName, wantLb, *expectedRule.Name)
			foundRule = true
		}
		if !foundRule {
			glog.V(10).Infof("reconcile(%s)(%t): lb rule(%s) adding", serviceName, wantLb, *expectedRule.Name)
			updatedRules = append(updatedRules, expectedRule)
			dirtyRules = true
		}
	}
	if dirtyRules {
		dirtyLb = true
		lb.LoadBalancingRules = &updatedRules
	}

	return lb, dirtyLb, nil
}

// This reconciles the Network Security Group similar to how the LB is reconciled.
// This entails adding required, missing SecurityRules and removing stale rules.
func (az *Cloud) reconcileSecurityGroup(sg network.SecurityGroup, clusterName string, service *v1.Service, wantLb bool) (network.SecurityGroup, bool, error) {
	serviceName := getServiceName(service)
	var ports []v1.ServicePort
	if wantLb {
		ports = service.Spec.Ports
	} else {
		ports = []v1.ServicePort{}
	}

	sourceRanges, err := serviceapi.GetLoadBalancerSourceRanges(service)
	if err != nil {
		return sg, false, err
	}
	var sourceAddressPrefixes []string
	if sourceRanges == nil || serviceapi.IsAllowAll(sourceRanges) {
		if !requiresInternalLoadBalancer(service) {
			sourceAddressPrefixes = []string{"Internet"}
		}
	} else {
		for _, ip := range sourceRanges {
			sourceAddressPrefixes = append(sourceAddressPrefixes, ip.String())
		}
	}
	expectedSecurityRules := make([]network.SecurityRule, len(ports)*len(sourceAddressPrefixes))

	for i, port := range ports {
		_, securityProto, _, err := getProtocolsFromKubernetesProtocol(port.Protocol)
		if err != nil {
			return sg, false, err
		}
		for j := range sourceAddressPrefixes {
			ix := i*len(sourceAddressPrefixes) + j
			securityRuleName := getSecurityRuleName(service, port, sourceAddressPrefixes[j])
			expectedSecurityRules[ix] = network.SecurityRule{
				Name: to.StringPtr(securityRuleName),
				SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
					Protocol:                 *securityProto,
					SourcePortRange:          to.StringPtr("*"),
					DestinationPortRange:     to.StringPtr(strconv.Itoa(int(port.Port))),
					SourceAddressPrefix:      to.StringPtr(sourceAddressPrefixes[j]),
					DestinationAddressPrefix: to.StringPtr("*"),
					Access:    network.Allow,
					Direction: network.Inbound,
				},
			}
		}
	}

	// update security rules
	dirtySg := false
	var updatedRules []network.SecurityRule
	if sg.SecurityRules != nil {
		updatedRules = *sg.SecurityRules
	}
	// update security rules: remove unwanted
	for i := len(updatedRules) - 1; i >= 0; i-- {
		existingRule := updatedRules[i]
		if serviceOwnsRule(service, *existingRule.Name) {
			glog.V(10).Infof("reconcile(%s)(%t): sg rule(%s) - considering evicting", serviceName, wantLb, *existingRule.Name)
			keepRule := false
			if findSecurityRule(expectedSecurityRules, existingRule) {
				glog.V(10).Infof("reconcile(%s)(%t): sg rule(%s) - keeping", serviceName, wantLb, *existingRule.Name)
				keepRule = true
			}
			if !keepRule {
				glog.V(10).Infof("reconcile(%s)(%t): sg rule(%s) - dropping", serviceName, wantLb, *existingRule.Name)
				updatedRules = append(updatedRules[:i], updatedRules[i+1:]...)
				dirtySg = true
			}
		}
	}
	// update security rules: add needed
	for _, expectedRule := range expectedSecurityRules {
		foundRule := false
		if findSecurityRule(updatedRules, expectedRule) {
			glog.V(10).Infof("reconcile(%s)(%t): sg rule(%s) - already exists", serviceName, wantLb, *expectedRule.Name)
			foundRule = true
		}
		if !foundRule {
			glog.V(10).Infof("reconcile(%s)(%t): sg rule(%s) - adding", serviceName, wantLb, *expectedRule.Name)

			nextAvailablePriority, err := getNextAvailablePriority(updatedRules)
			if err != nil {
				return sg, false, err
			}

			expectedRule.Priority = to.Int32Ptr(nextAvailablePriority)
			updatedRules = append(updatedRules, expectedRule)
			dirtySg = true
		}
	}
	if dirtySg {
		sg.SecurityRules = &updatedRules
	}
	return sg, dirtySg, nil
}

func findProbe(probes []network.Probe, probe network.Probe) bool {
	for _, existingProbe := range probes {
		if strings.EqualFold(*existingProbe.Name, *probe.Name) {
			return true
		}
	}
	return false
}

func findRule(rules []network.LoadBalancingRule, rule network.LoadBalancingRule) bool {
	for _, existingRule := range rules {
		if strings.EqualFold(*existingRule.Name, *rule.Name) {
			return true
		}
	}
	return false
}

func findSecurityRule(rules []network.SecurityRule, rule network.SecurityRule) bool {
	for _, existingRule := range rules {
		if strings.EqualFold(*existingRule.Name, *rule.Name) {
			return true
		}
	}
	return false
}

// This ensures the given VM's Primary NIC's Primary IP Configuration is
// participating in the specified LoadBalancer Backend Pool.
func (az *Cloud) ensureHostInPool(serviceName string, nodeName types.NodeName, backendPoolID string) error {
	vmName := mapNodeNameToVMName(nodeName)
	machine, err := az.VirtualMachinesClient.Get(az.ResourceGroup, vmName, "")
	if err != nil {
		return err
	}

	primaryNicID, err := getPrimaryInterfaceID(machine)
	if err != nil {
		return err
	}
	nicName, err := getLastSegment(primaryNicID)
	if err != nil {
		return err
	}

	// Check availability set
	if az.PrimaryAvailabilitySetName != "" {
		expectedAvailabilitySetName := az.getAvailabilitySetID(az.PrimaryAvailabilitySetName)
		if !strings.EqualFold(*machine.AvailabilitySet.ID, expectedAvailabilitySetName) {
			glog.V(3).Infof(
				"nicupdate(%s): skipping nic (%s) since it is not in the primaryAvailabilitSet(%s)",
				serviceName, nicName, az.PrimaryAvailabilitySetName)
			return nil
		}
	}

	nic, err := az.InterfacesClient.Get(az.ResourceGroup, nicName, "")
	if err != nil {
		return err
	}

	var primaryIPConfig *network.InterfaceIPConfiguration
	primaryIPConfig, err = getPrimaryIPConfig(nic)
	if err != nil {
		return err
	}

	foundPool := false
	newBackendPools := []network.BackendAddressPool{}
	if primaryIPConfig.LoadBalancerBackendAddressPools != nil {
		newBackendPools = *primaryIPConfig.LoadBalancerBackendAddressPools
	}
	for _, existingPool := range newBackendPools {
		if strings.EqualFold(backendPoolID, *existingPool.ID) {
			foundPool = true
			break
		}
	}
	if !foundPool {
		newBackendPools = append(newBackendPools,
			network.BackendAddressPool{
				ID: to.StringPtr(backendPoolID),
			})

		primaryIPConfig.LoadBalancerBackendAddressPools = &newBackendPools

		glog.V(3).Infof("nicupdate(%s): nic(%s) - updating", serviceName, nicName)
		_, err := az.InterfacesClient.CreateOrUpdate(az.ResourceGroup, *nic.Name, nic, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

// Check if service requires an internal load balancer.
func requiresInternalLoadBalancer(service *v1.Service) bool {
	if l, ok := service.Annotations[ServiceAnnotationLoadBalancerInternal]; ok {
		return l == "true"
	}

	return false
}
