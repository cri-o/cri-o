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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/knftables"
)

var expectedNFTablesElementsV4 = []string{
	`add element ip crio-hostports hostports { tcp . 8080 comment "VEBO64P7B2WCUAON" : 10.1.1.2 . 80 }`,
	`add element ip crio-hostports hostports { udp . 8081 comment "VEBO64P7B2WCUAON" : 10.1.1.2 . 81 }`,
	`add element ip crio-hostports hostports { sctp . 8083 comment "VEBO64P7B2WCUAON" : 10.1.1.2 . 83 }`,
	`add element ip crio-hostports hostipports { 127.0.0.1 . tcp . 8084 comment "VEBO64P7B2WCUAON" : 10.1.1.2 . 84 }`,
	`add element ip crio-hostports hairpins { 10.1.1.2 . 10.1.1.2 comment "VEBO64P7B2WCUAON" }`,

	`add element ip crio-hostports hostports { tcp . 8443 comment "XDYUBNL7AIWQOXRB" : 10.1.1.4 . 443 }`,
	`add element ip crio-hostports hairpins { 10.1.1.4 . 10.1.1.4 comment "XDYUBNL7AIWQOXRB" }`,

	`add element ip crio-hostports hostipports { 127.0.0.1 . tcp . 8888 comment "7HJJ4B4NBF4ZM5U2" : 10.1.1.5 . 443 }`,
	`add element ip crio-hostports hostipports { 127.0.0.2 . tcp . 8888 comment "7HJJ4B4NBF4ZM5U2" : 10.1.1.5 . 443 }`,
	`add element ip crio-hostports hairpins { 10.1.1.5 . 10.1.1.5 comment "7HJJ4B4NBF4ZM5U2" }`,

	`add element ip crio-hostports hostports { tcp . 9999 comment "4QZEO3QNGY7ECK5Q" : 10.1.1.6 . 443 }`,
	`add element ip crio-hostports hostports { udp . 9999 comment "4QZEO3QNGY7ECK5Q" : 10.1.1.6 . 443 }`,
	`add element ip crio-hostports hairpins { 10.1.1.6 . 10.1.1.6 comment "4QZEO3QNGY7ECK5Q" }`,
}

var expectedNFTablesElementsV6 = []string{
	`add element ip6 crio-hostports hostports { tcp . 8080 comment "VEBO64P7B2WCUAON" : 2001:beef::2 . 80 }`,
	`add element ip6 crio-hostports hostports { udp . 8081 comment "VEBO64P7B2WCUAON" : 2001:beef::2 . 81 }`,
	`add element ip6 crio-hostports hostports { sctp . 8083 comment "VEBO64P7B2WCUAON" : 2001:beef::2 . 83 }`,
	`add element ip6 crio-hostports hostipports { ::1 . tcp . 8084 comment "VEBO64P7B2WCUAON" : 2001:beef::2 . 84 }`,
	`add element ip6 crio-hostports hairpins { 2001:beef::2 . 2001:beef::2 comment "VEBO64P7B2WCUAON" }`,

	`add element ip6 crio-hostports hostports { tcp . 8443 comment "XDYUBNL7AIWQOXRB" : 2001:beef::4 . 443 }`,
	`add element ip6 crio-hostports hairpins { 2001:beef::4 . 2001:beef::4 comment "XDYUBNL7AIWQOXRB" }`,
}

func checkNFTablesElements(nft *knftables.Fake, expectedElements []string) {
	dump := nft.Dump()

	expected := sets.New(expectedElements...)

	matched := sets.New[string]()

	for line := range strings.SplitSeq(dump, "\n") {
		if strings.HasPrefix(line, "add element") {
			matched.Insert(line)
		}
	}

	unexpectedElements := matched.Difference(expected).UnsortedList()
	missingElements := expected.Difference(matched).UnsortedList()

	ExpectWithOffset(1, len(unexpectedElements)+len(missingElements)).To(Equal(0), "unexpected elements in nftables dump: %#v, expected elements missing from nftables dump: %#v", unexpectedElements, missingElements)
}

var _ = t.Describe("hostPortManagerNFTables", func() {
	It("should ensure hostports table", func() {
		fakeNFT := knftables.NewFake(knftables.IPv4Family, hostPortsTable)
		tx := fakeNFT.NewTransaction()
		ensureHostPortsTable(tx, knftables.IPv4Family)
		Expect(fakeNFT.Run(context.Background(), tx)).To(Succeed())

		dump := fakeNFT.Dump()
		expected := `
add table ip crio-hostports { comment "HostPort rules created by CRI-O" ; }
add chain ip crio-hostports hostports
add chain ip crio-hostports masquerading { type nat hook postrouting priority 100 ; }
add chain ip crio-hostports output { type nat hook output priority -100 ; }
add chain ip crio-hostports prerouting { type nat hook prerouting priority -100 ; }
add set ip crio-hostports hairpins { type ipv4_addr . ipv4_addr ; comment "hostport hairpin connections" ; }
add map ip crio-hostports hostipports { type ipv4_addr . inet_proto . inet_service : ipv4_addr . inet_service ; comment "hostports on specific IPs (hostIP . protocol . hostPort -> podIP . podPort)" ; }
add map ip crio-hostports hostports { type inet_proto . inet_service : ipv4_addr . inet_service ; comment "hostports on all local IPs (protocol . hostPort -> podIP . podPort)" ; }
add rule ip crio-hostports hostports dnat ip addr . port to ip daddr . meta l4proto . th dport map @hostipports
add rule ip crio-hostports hostports dnat ip addr . port to meta l4proto . th dport map @hostports
add rule ip crio-hostports masquerading ct status & dnat|snat == dnat ip saddr . ip daddr @hairpins masquerade
add rule ip crio-hostports output fib daddr type local  goto hostports
add rule ip crio-hostports prerouting fib daddr type local  goto hostports
`
		Expect(strings.TrimSpace(dump)).To(Equal(strings.TrimSpace(expected)))
	})

	It("should support IPv4", func() {
		fakeNFT := knftables.NewFake(knftables.IPv4Family, hostPortsTable)
		manager := &hostportManagerNFTables{
			nft:    fakeNFT,
			family: knftables.IPv4Family,
		}

		// Add Hostports
		for _, tc := range testCasesV4 {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check nftables after adding hostports
		checkNFTablesElements(fakeNFT, expectedNFTablesElementsV4)

		// Remove all added hostports
		for _, tc := range testCasesV4 {
			err := manager.Remove(tc.id, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check nftables after deleting hostports
		checkNFTablesElements(fakeNFT, nil)
	})

	It("should support IPv6", func() {
		fakeNFT := knftables.NewFake(knftables.IPv6Family, hostPortsTable)
		manager := &hostportManagerNFTables{
			nft:    fakeNFT,
			family: knftables.IPv6Family,
		}

		// Add Hostports
		for _, tc := range testCasesV6 {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check nftables after adding hostports
		checkNFTablesElements(fakeNFT, expectedNFTablesElementsV6)

		// Remove all added hostports
		for _, tc := range testCasesV6 {
			err := manager.Remove(tc.id, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check nftables after deleting hostports
		checkNFTablesElements(fakeNFT, nil)
	})
})
