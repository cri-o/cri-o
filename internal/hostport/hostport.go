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
	"fmt"
	"net"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	utiliptables "github.com/cri-o/cri-o/internal/iptables"
)

const (
	// the hostport chain.
	kubeHostportsChain utiliptables.Chain = "KUBE-HOSTPORTS"
	// prefix for hostport chains.
	kubeHostportChainPrefix string = "KUBE-HP-"

	// the masquerade chain.
	crioMasqueradeChain utiliptables.Chain = "CRIO-HOSTPORTS-MASQ"
	// prefix for masquerade chains.
	crioMasqueradeChainPrefix string = "CRIO-MASQ-"
)

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

// ensureKubeHostportChains ensures the KUBE-HOSTPORTS chain is setup correctly.
func ensureKubeHostportChains(iptables utiliptables.Interface) error {
	logrus.Info("Ensuring kubelet hostport chains")
	// Ensure kubeHostportChain
	if _, err := iptables.EnsureChain(utiliptables.TableNAT, kubeHostportsChain); err != nil {
		return fmt.Errorf("failed to ensure that %s chain %s exists: %w", utiliptables.TableNAT, kubeHostportsChain, err)
	}

	tableChainsNeedJumpServices := []struct {
		table utiliptables.Table
		chain utiliptables.Chain
	}{
		{utiliptables.TableNAT, utiliptables.ChainOutput},
		{utiliptables.TableNAT, utiliptables.ChainPrerouting},
	}
	args := []string{
		"-m", "comment", "--comment", "kube hostport portals",
		"-m", "addrtype", "--dst-type", "LOCAL",
		"-j", string(kubeHostportsChain),
	}

	for _, tc := range tableChainsNeedJumpServices {
		// KUBE-HOSTPORTS chain needs to be appended to the system chains.
		// This ensures KUBE-SERVICES chain gets processed first.
		// Since rules in KUBE-HOSTPORTS chain matches broader cases, allow the more specific rules to be processed first.
		if _, err := iptables.EnsureRule(utiliptables.Append, tc.table, tc.chain, args...); err != nil {
			return fmt.Errorf("failed to ensure that %s chain %s jumps to %s: %w", tc.table, tc.chain, kubeHostportsChain, err)
		}
	}

	// Ensure crioMasqueradeChain
	if _, err := iptables.EnsureChain(utiliptables.TableNAT, crioMasqueradeChain); err != nil {
		return fmt.Errorf("failed to ensure that %s chain %s exists: %w", utiliptables.TableNAT, crioMasqueradeChain, err)
	}

	args = []string{
		"-m", "comment", "--comment", "kube hostport masquerading",
		"-m", "conntrack", "--ctstate", "DNAT",
		"-j", string(crioMasqueradeChain),
	}
	if _, err := iptables.EnsureRule(utiliptables.Append, utiliptables.TableNAT, utiliptables.ChainPostrouting, args...); err != nil {
		return fmt.Errorf("failed to ensure that %s chain %s jumps to %s: %w", utiliptables.TableNAT, utiliptables.ChainPostrouting, crioMasqueradeChain, err)
	}

	return nil
}
