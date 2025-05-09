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
	nft    knftables.Interface
	family knftables.Family
	mu     sync.Mutex
}

// newHostportManagerNFTables creates a new nftables HostPortManager.
func newHostportManagerNFTables(family knftables.Family) (*hostportManagerNFTables, error) {
	nft, err := knftables.New(family, hostPortsTable)
	if err != nil {
		return nil, err
	}

	return &hostportManagerNFTables{
		nft:    nft,
		family: family,
	}, nil
}

func (hm *hostportManagerNFTables) Add(id, name, podIP string, hostportMappings []*PortMapping) (err error) {
	// Ensure atomicity for nftables operations
	hm.mu.Lock()
	defer hm.mu.Unlock()

	tx := hm.nft.NewTransaction()
	ensureHostPortsTable(tx, hm.family)

	// Add map/set elements to tx for all mappings in hostportMappings. We add a
	// comment to each element based on the sandbox ID, so we can match them up to
	// this pod in Remove().
	comment := hashSandboxID(id)

	for _, pm := range hostportMappings {
		protocol := strings.ToLower(string(pm.Protocol))
		hostPort := strconv.Itoa(int(pm.HostPort))
		containerPort := strconv.Itoa(int(pm.ContainerPort))

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

	// Add a hairpinning mapping.
	tx.Add(&knftables.Element{
		Set: hairpinSet,
		Key: []string{
			podIP, podIP,
		},
		Comment: &comment,
	})

	err = hm.nft.Run(context.TODO(), tx)
	if err != nil {
		return fmt.Errorf("failed to ensure nftables chains: %w", err)
	}

	return nil
}

func (hm *hostportManagerNFTables) Remove(id string, hostportMappings []*PortMapping) (err error) {
	// Ensure atomicity for nftables operations
	hm.mu.Lock()
	defer hm.mu.Unlock()

	comment := hashSandboxID(id)

	// Fetch the existing map/set elements.
	existingHostPorts, err := hm.nft.ListElements(context.TODO(), "map", hostPortsMap)
	if err != nil && !knftables.IsNotFound(err) {
		return fmt.Errorf("could not list existing hostports: %w", err)
	}

	existingHostIPPorts, err := hm.nft.ListElements(context.TODO(), "map", hostIPPortsMap)
	if err != nil && !knftables.IsNotFound(err) {
		return fmt.Errorf("could not list existing hostports: %w", err)
	}

	existingHairpins, err := hm.nft.ListElements(context.TODO(), "set", hairpinSet)
	if err != nil && !knftables.IsNotFound(err) {
		return fmt.Errorf("could not list existing hostports: %w", err)
	}

	// Delete each one that refers to this pod in its Comment.
	tx := hm.nft.NewTransaction()

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

	err = hm.nft.Run(context.TODO(), tx)
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
func ensureHostPortsTable(tx *knftables.Transaction, family knftables.Family) {
	ip := "ip"
	ipaddr := "ipv4_addr"

	if family == knftables.IPv6Family {
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
}
