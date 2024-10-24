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
	"context"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilnet "k8s.io/utils/net"
	"sigs.k8s.io/knftables"
)

const (
	// our nftables table.
	hostPortsTable string = "crio-hostports"

	// maps and sets referred to from HostportManager.
	hostPortsMap   string = "hostports"
	hostIPPortsMap string = "hostipports"
	hairpinSet     string = "hairpins"
)

type hostportManagerNFTables struct {
	nft4 knftables.Interface
	nft6 knftables.Interface
	err  error
	mu   sync.Mutex
}

// newHostportManagerNFTables creates a new nftables HostPortManager.
func newHostportManagerNFTables() (HostPortManager, error) {
	nft4, err := knftables.New(knftables.IPv4Family, hostPortsTable)
	if err != nil {
		return nil, err
	}
	nft6, err := knftables.New(knftables.IPv6Family, hostPortsTable)
	if err != nil {
		return nil, err
	}

	return &hostportManagerNFTables{
		nft4: nft4,
		nft6: nft6,
	}, nil
}

func (hm *hostportManagerNFTables) Add(id string, podPortMapping *PodPortMapping, natInterfaceName string) (err error) {
	podFullName := getPodFullName(podPortMapping)
	// IP.To16() returns nil if IP is not a valid IPv4 or IPv6 address
	if podPortMapping.IP.To16() == nil {
		return fmt.Errorf("invalid or missing IP of pod %s", podFullName)
	}
	podIP := podPortMapping.IP.String()
	isIPv6 := utilnet.IsIPv6(podPortMapping.IP)

	// skip if there is no hostport needed
	hostportMappings := gatherHostportMappings(podPortMapping, isIPv6)
	if len(hostportMappings) == 0 {
		return nil
	}

	if hm.err != nil {
		return hm.err
	}

	var nft knftables.Interface
	if isIPv6 {
		nft = hm.nft6
	} else {
		nft = hm.nft4
	}

	// Ensure atomicity for nftables operations
	hm.mu.Lock()
	defer hm.mu.Unlock()

	tx := nft.NewTransaction()
	ensureHostPortsTable(tx, natInterfaceName, isIPv6)

	// Add map/set elements to tx for all mappings in hostportMappings. We add a
	// comment to each element based on the sandbox ID, so we can match them up to
	// this pod in Remove().
	comment := hashSandboxID(id)
	conntrackPortsToRemove := []int{}
	for _, pm := range hostportMappings {
		protocol := strings.ToLower(string(pm.Protocol))
		hostPort := strconv.Itoa(int(pm.HostPort))
		containerPort := strconv.Itoa(int(pm.ContainerPort))
		if pm.Protocol == v1.ProtocolUDP {
			conntrackPortsToRemove = append(conntrackPortsToRemove, int(pm.HostPort))
		}

		if pm.HostIP == "" || pm.HostIP == "0.0.0.0" || pm.HostIP == "::" {
			tx.Add(&knftables.Element{
				Map: hostPortsMap,
				Key: []string{
					protocol, hostPort,
				},
				Value: []string{
					podIP, containerPort,
				},
				Comment: &comment,
			})
		} else {
			tx.Add(&knftables.Element{
				Map: hostIPPortsMap,
				Key: []string{
					pm.HostIP, protocol, hostPort,
				},
				Value: []string{
					podIP, containerPort,
				},
				Comment: &comment,
			})
		}
	}

	// If we added any hostport mappings, add a hairpinning mapping.
	if tx.NumOperations() != 0 {
		tx.Add(&knftables.Element{
			Set: hairpinSet,
			Key: []string{
				podIP, podIP,
			},
			Comment: &comment,
		})
	}

	err = nft.Run(context.TODO(), tx)
	if err != nil {
		return fmt.Errorf("failed to ensure nftables chains: %w", err)
	}

	// Remove conntrack entries just after updating the nftables sets. If the
	// conntrack entry is removed before the nftables update, it can be the case that
	// the packets received by the node after nftables update will create a new
	// conntrack entry without any DNAT. That will result in blackhole of the traffic
	// even after correct nftables rules have been added back.
	logrus.Infof("Starting to delete udp conntrack entries: %v, isIPv6 - %v", conntrackPortsToRemove, isIPv6)
	// https://www.iana.org/assignments/protocol-numbers/protocol-numbers.xhtml
	const protocolUDPNumber = 17
	for _, port := range conntrackPortsToRemove {
		err = deleteConntrackEntriesForDstPort(uint16(port), protocolUDPNumber, getNetlinkFamily(isIPv6))
		if err != nil {
			logrus.Errorf("Failed to clear udp conntrack for port %d, error: %v", port, err)
		}
	}
	return nil
}

func (hm *hostportManagerNFTables) Remove(id string, podPortMapping *PodPortMapping) (err error) {
	if hm.err != nil {
		return hm.err
	}

	var errors []error
	// Remove may not have the IP information, so we try to clean us much as possible
	// and warn about the possible errors
	err = hm.removeForFamily(id, podPortMapping, hm.nft4)
	if err != nil {
		errors = append(errors, err)
	}
	err = hm.removeForFamily(id, podPortMapping, hm.nft6)
	if err != nil {
		errors = append(errors, err)
	}

	return utilerrors.NewAggregate(errors)
}

func (hm *hostportManagerNFTables) removeForFamily(id string, podPortMapping *PodPortMapping, nft knftables.Interface) (err error) {
	isIPv6 := nft == hm.nft6
	hostportMappings := gatherHostportMappings(podPortMapping, isIPv6)
	if len(hostportMappings) == 0 {
		return nil
	}
	comment := hashSandboxID(id)

	// Ensure atomicity for nftables operations
	hm.mu.Lock()
	defer hm.mu.Unlock()

	// Fetch the existing map/set elements.
	existingHostPorts, err := nft.ListElements(context.TODO(), "map", hostPortsMap)
	if err != nil && !knftables.IsNotFound(err) {
		return fmt.Errorf("could not list existing hostports: %w", err)
	}
	existingHostIPPorts, err := nft.ListElements(context.TODO(), "map", hostIPPortsMap)
	if err != nil && !knftables.IsNotFound(err) {
		return fmt.Errorf("could not list existing hostports: %w", err)
	}
	existingHairpins, err := nft.ListElements(context.TODO(), "set", hairpinSet)
	if err != nil && !knftables.IsNotFound(err) {
		return fmt.Errorf("could not list existing hostports: %w", err)
	}

	// Delete each one that refers to this pod in its Comment.
	tx := nft.NewTransaction()
	for _, elem := range existingHostPorts {
		if elem.Comment != nil && *elem.Comment == comment {
			tx.Delete(elem)
		}
	}
	for _, elem := range existingHostIPPorts {
		if elem.Comment != nil && *elem.Comment == comment {
			tx.Delete(elem)
		}
	}
	for _, elem := range existingHairpins {
		if elem.Comment != nil && *elem.Comment == comment {
			tx.Delete(elem)
		}
	}
	if tx.NumOperations() == 0 {
		return nil
	}

	err = nft.Run(context.TODO(), tx)
	if err != nil {
		return fmt.Errorf("failed to clean up nftables hostport maps: %w", err)
	}
	return nil
}

// hashSandboxID hashes the sandbox ID to get a suitable identifier for an nftables
// comment (which must be at most 128 characters).
func hashSandboxID(id string) string {
	hash := sha256.Sum256([]byte(id))
	encoded := base32.StdEncoding.EncodeToString(hash[:])
	return encoded[:16]
}

// ensureHostPortsTable adds rules to tx to ensure the hostPortsTable is setup correctly.
// "tx.Add" silently no-ops if the object already exists, so after the first hostport is
// added, this function won't really do anything. (Note that for the chains (which have a
// static set of rules), we Add+Flush the chain and then Add the desired rules, thus
// ensuring that the chain has exactly the rules we want, while for the maps and set
// (which have per-hostport elements), we Add them but don't modify their contents.)
func ensureHostPortsTable(tx *knftables.Transaction, natInterfaceName string, isIPv6 bool) {
	ip := "ip"
	ipaddr := "ipv4_addr"
	if isIPv6 {
		ip = "ip6"
		ipaddr = "ipv6_addr"
	}

	tx.Add(&knftables.Table{
		Comment: knftables.PtrTo("HostPort rules created by CRI-O"),
	})

	tx.Add(&knftables.Map{
		Name: hostPortsMap,
		Type: knftables.Concat(
			"inet_proto", ".", "inet_service", ":", ipaddr, ".", "inet_service",
		),
		Comment: knftables.PtrTo("hostports on all local IPs (protocol . hostPort -> podIP . podPort)"),
	})
	tx.Add(&knftables.Map{
		Name: hostIPPortsMap,
		Type: knftables.Concat(
			ipaddr, ".", "inet_proto", ".", "inet_service", ":", ipaddr, ".", "inet_service",
		),
		Comment: knftables.PtrTo("hostports on specific IPs (hostIP . protocol . hostPort -> podIP . podPort)"),
	})

	// Create the "hostports" chain with the map lookup rules, and then create
	// "prerouting" and "output" chains that call the "hostports" chain for
	// locally-destined packets.
	tx.Add(&knftables.Chain{
		Name: "hostports",
	})
	tx.Flush(&knftables.Chain{
		Name: "hostports",
	})
	// hostIPPortsMap check must come first since the hostPortsMap rule catches all IPs
	tx.Add(&knftables.Rule{
		Chain: "hostports",
		Rule: knftables.Concat(
			"dnat", ip, "addr . port to",
			ip, "daddr", ".", "meta l4proto", ".", "th dport", "map", "@", hostIPPortsMap,
		),
	})
	tx.Add(&knftables.Rule{
		Chain: "hostports",
		Rule: knftables.Concat(
			"dnat", ip, "addr . port to",
			"meta l4proto", ".", "th dport", "map", "@", hostPortsMap,
		),
	})

	tx.Add(&knftables.Chain{
		Name:     "prerouting",
		Type:     knftables.PtrTo(knftables.NATType),
		Hook:     knftables.PtrTo(knftables.PreroutingHook),
		Priority: knftables.PtrTo(knftables.DNATPriority),
	})
	tx.Flush(&knftables.Chain{
		Name: "prerouting",
	})
	tx.Add(&knftables.Rule{
		Chain: "prerouting",
		Rule:  "fib daddr type local  goto hostports",
	})

	tx.Add(&knftables.Chain{
		Name:     "output",
		Type:     knftables.PtrTo(knftables.NATType),
		Hook:     knftables.PtrTo(knftables.OutputHook),
		Priority: knftables.PtrTo(knftables.DNATPriority),
	})
	tx.Flush(&knftables.Chain{
		Name: "output",
	})
	tx.Add(&knftables.Rule{
		Chain: "output",
		Rule:  "fib daddr type local  goto hostports",
	})

	// Create the "masquerading" chain, linked to the postrouting hook.
	tx.Add(&knftables.Chain{
		Name:     "masquerading",
		Type:     knftables.PtrTo(knftables.NATType),
		Hook:     knftables.PtrTo(knftables.PostroutingHook),
		Priority: knftables.PtrTo(knftables.SNATPriority),
	})
	tx.Flush(&knftables.Chain{
		Name: "masquerading",
	})
	// You can't write an nftables rule that checks "source IP == destination IP" so
	// instead we match it by looking up the source and destination IP in a set that
	// has been filled in (by HostportManager) with entries containing the same pod IP
	// twice. We can't _exactly_ match only the traffic that was definitely DNATted by
	// our rule as opposed to someone else's, but if the traffic has been DNATted and
	// has src=dst=podIP then _someone_ needs to masquerade it.
	tx.Add(&knftables.Set{
		Name: hairpinSet,
		Type: knftables.Concat(
			ipaddr, ".", ipaddr,
		),
		Comment: knftables.PtrTo("hostport hairpin connections"),
	})
	// Note that this rule runs after any "dnat" rules in "hostports", so "ip daddr"
	// here is the DNATted destination address (the pod IP), not the original packet's
	// destination address.
	tx.Add(&knftables.Rule{
		Chain: "masquerading",
		Rule: knftables.Concat(
			"ct", "status", "&", "dnat|snat", "==", "dnat",
			ip, "saddr", ".", ip, "daddr", "@", hairpinSet,
			"masquerade",
		),
	})
	if natInterfaceName != "" && natInterfaceName != "lo" {
		// Need to SNAT traffic from localhost
		if isIPv6 {
			tx.Add(&knftables.Rule{
				Chain: "masquerading",
				Rule:  fmt.Sprintf("ip6 saddr ::1 oifname %q masquerade", natInterfaceName),
			})
		} else {
			tx.Add(&knftables.Rule{
				Chain: "masquerading",
				Rule:  fmt.Sprintf("ip saddr 127.0.0.0/8 oifname %q masquerade", natInterfaceName),
			})
		}
	}
}
