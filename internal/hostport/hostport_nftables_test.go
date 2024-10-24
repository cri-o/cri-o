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
	"net"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/knftables"
)

// newFakeManagerNFTables creates a new Manager with fake knftables. Note that we need to
// create both ipv4 and ipv6 even for the single-stack tests, because Remove() will try to
// use both.
func newFakeManagerNFTables() (manager *hostportManagerNFTables, nft4, nft6 *knftables.Fake) {
	nft4 = knftables.NewFake(knftables.IPv4Family, hostPortsTable)
	nft6 = knftables.NewFake(knftables.IPv6Family, hostPortsTable)
	return &hostportManagerNFTables{nft4: nft4, nft6: nft6}, nft4, nft6
}

var _ = t.Describe("HostPortManagerNFTables", func() {
	It("should ensure hostports table", func() {
		fakeNFT := knftables.NewFake(knftables.IPv4Family, hostPortsTable)
		tx := fakeNFT.NewTransaction()
		ensureHostPortsTable(tx, "cbr0", false)
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
add rule ip crio-hostports masquerading ip saddr 127.0.0.0/8 oifname "cbr0" masquerade
add rule ip crio-hostports output fib daddr type local  goto hostports
add rule ip crio-hostports prerouting fib daddr type local  goto hostports
`
		Expect(strings.TrimSpace(dump)).To(Equal(strings.TrimSpace(expected)))
	})

	It("should support IPv4", func() {
		manager, nft4, _ := newFakeManagerNFTables()
		testCases := []struct {
			mapping     *PodPortMapping
			expectError bool
		}{
			// open HostPorts 8080/TCP, 8081/UDP and 8083/SCTP
			{
				mapping: &PodPortMapping{
					Name:      "pod1",
					Namespace: "ns1",
					IP:        net.ParseIP("10.1.1.2"),
					PortMappings: []*PortMapping{
						{
							HostPort:      8080,
							ContainerPort: 80,
							Protocol:      v1.ProtocolTCP,
						},
						{
							HostPort:      8081,
							ContainerPort: 81,
							Protocol:      v1.ProtocolUDP,
						},
						{
							HostPort:      8083,
							ContainerPort: 83,
							Protocol:      v1.ProtocolSCTP,
						},
					},
				},
				expectError: false,
			},
			// open port 443
			{
				mapping: &PodPortMapping{
					Name:      "pod3",
					Namespace: "ns1",
					IP:        net.ParseIP("10.1.1.4"),
					PortMappings: []*PortMapping{
						{
							HostPort:      8443,
							ContainerPort: 443,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
				expectError: false,
			},
			// open same HostPort on different IP
			{
				mapping: &PodPortMapping{
					Name:      "pod5",
					Namespace: "ns5",
					IP:        net.ParseIP("10.1.1.5"),
					PortMappings: []*PortMapping{
						{
							HostPort:      8888,
							ContainerPort: 443,
							Protocol:      v1.ProtocolTCP,
							HostIP:        "127.0.0.2",
						},
						{
							HostPort:      8888,
							ContainerPort: 443,
							Protocol:      v1.ProtocolTCP,
							HostIP:        "127.0.0.1",
						},
					},
				},
				expectError: false,
			},
			// open same HostPort on different
			{
				mapping: &PodPortMapping{
					Name:      "pod6",
					Namespace: "ns1",
					IP:        net.ParseIP("10.1.1.6"),
					PortMappings: []*PortMapping{
						{
							HostPort:      9999,
							ContainerPort: 443,
							Protocol:      v1.ProtocolTCP,
						},
						{
							HostPort:      9999,
							ContainerPort: 443,
							Protocol:      v1.ProtocolUDP,
						},
					},
				},
				expectError: false,
			},
		}

		// Add Hostports
		for _, tc := range testCases {
			sandboxID := getPodFullName(tc.mapping)
			err := manager.Add(sandboxID, tc.mapping, "cbr0")
			if tc.expectError {
				Expect(err).To(HaveOccurred())
				continue
			}
			Expect(err).NotTo(HaveOccurred())
		}

		// Check nftables after adding hostports
		checkElements(nft4, []string{
			`add element ip crio-hostports hostports { tcp . 8080 comment "BBK5KOLM3RTTV4JS" : 10.1.1.2 . 80 }`,
			`add element ip crio-hostports hostports { udp . 8081 comment "BBK5KOLM3RTTV4JS" : 10.1.1.2 . 81 }`,
			`add element ip crio-hostports hostports { sctp . 8083 comment "BBK5KOLM3RTTV4JS" : 10.1.1.2 . 83 }`,
			`add element ip crio-hostports hairpins { 10.1.1.2 . 10.1.1.2 comment "BBK5KOLM3RTTV4JS" }`,

			`add element ip crio-hostports hostports { tcp . 8443 comment "FWUCPWRIB7ZR62ZF" : 10.1.1.4 . 443 }`,
			`add element ip crio-hostports hairpins { 10.1.1.4 . 10.1.1.4 comment "FWUCPWRIB7ZR62ZF" }`,

			`add element ip crio-hostports hostipports { 127.0.0.2 . tcp . 8888 comment "6UOYUYR5DU6TDVSV" : 10.1.1.5 . 443 }`,
			`add element ip crio-hostports hostipports { 127.0.0.1 . tcp . 8888 comment "6UOYUYR5DU6TDVSV" : 10.1.1.5 . 443 }`,
			`add element ip crio-hostports hairpins { 10.1.1.5 . 10.1.1.5 comment "6UOYUYR5DU6TDVSV" }`,

			`add element ip crio-hostports hostports { tcp . 9999 comment "VJVSBXBJ2B2XAD5F" : 10.1.1.6 . 443 }`,
			`add element ip crio-hostports hostports { udp . 9999 comment "VJVSBXBJ2B2XAD5F" : 10.1.1.6 . 443 }`,
			`add element ip crio-hostports hairpins { 10.1.1.6 . 10.1.1.6 comment "VJVSBXBJ2B2XAD5F" }`,
		})

		// Remove all added hostports
		for _, tc := range testCases {
			if !tc.expectError {
				sandboxID := getPodFullName(tc.mapping)
				err := manager.Remove(sandboxID, tc.mapping)
				Expect(err).NotTo(HaveOccurred())
			}
		}

		// Check nftables after deleting hostports
		checkElements(nft4, []string{})
	})

	It("should support IPv6", func() {
		manager, _, nft6 := newFakeManagerNFTables()
		testCases := []struct {
			mapping     *PodPortMapping
			expectError bool
		}{
			{
				mapping: &PodPortMapping{
					Name:      "pod1",
					Namespace: "ns1",
					IP:        net.ParseIP("2001:beef::2"),
					PortMappings: []*PortMapping{
						{
							HostPort:      8080,
							ContainerPort: 80,
							Protocol:      v1.ProtocolTCP,
						},
						{
							HostPort:      8081,
							ContainerPort: 81,
							Protocol:      v1.ProtocolUDP,
						},
						{
							HostPort:      8083,
							ContainerPort: 83,
							Protocol:      v1.ProtocolSCTP,
						},
					},
				},
				expectError: false,
			},
			{
				mapping: &PodPortMapping{
					Name:      "pod3",
					Namespace: "ns1",
					IP:        net.ParseIP("2001:beef::4"),
					PortMappings: []*PortMapping{
						{
							HostPort:      8443,
							ContainerPort: 443,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
				expectError: false,
			},
		}

		// Add Hostports
		for _, tc := range testCases {
			sandboxID := getPodFullName(tc.mapping)
			err := manager.Add(sandboxID, tc.mapping, "cbr0")
			if tc.expectError {
				Expect(err).To(HaveOccurred())
				continue
			}
			Expect(err).NotTo(HaveOccurred())
		}

		// Check nftables after adding hostports
		checkElements(nft6, []string{
			`add element ip6 crio-hostports hostports { tcp . 8080 comment "BBK5KOLM3RTTV4JS" : 2001:beef::2 . 80 }`,
			`add element ip6 crio-hostports hostports { udp . 8081 comment "BBK5KOLM3RTTV4JS" : 2001:beef::2 . 81 }`,
			`add element ip6 crio-hostports hostports { sctp . 8083 comment "BBK5KOLM3RTTV4JS" : 2001:beef::2 . 83 }`,
			`add element ip6 crio-hostports hairpins { 2001:beef::2 . 2001:beef::2 comment "BBK5KOLM3RTTV4JS" }`,

			`add element ip6 crio-hostports hostports { tcp . 8443 comment "FWUCPWRIB7ZR62ZF" : 2001:beef::4 . 443 }`,
			`add element ip6 crio-hostports hairpins { 2001:beef::4 . 2001:beef::4 comment "FWUCPWRIB7ZR62ZF" }`,
		})

		// Remove all added hostports
		for _, tc := range testCases {
			if !tc.expectError {
				sandboxID := getPodFullName(tc.mapping)
				err := manager.Remove(sandboxID, tc.mapping)
				Expect(err).NotTo(HaveOccurred())
			}
		}

		// Check nftables after deleting hostports
		checkElements(nft6, []string{})
	})

	It("should support dual stack", func() {
		manager, nft4, nft6 := newFakeManagerNFTables()
		testCases := []struct {
			mapping     *PodPortMapping
			expectError bool
		}{
			{
				mapping: &PodPortMapping{
					Name:      "pod1",
					Namespace: "ns1",
					IP:        net.ParseIP("192.168.2.7"),
					PortMappings: []*PortMapping{
						{
							HostPort:      8080,
							ContainerPort: 80,
							Protocol:      v1.ProtocolTCP,
						},
						{
							HostPort:      8081,
							ContainerPort: 81,
							Protocol:      v1.ProtocolUDP,
						},
						{
							HostPort:      8083,
							ContainerPort: 83,
							Protocol:      v1.ProtocolSCTP,
						},
						{
							HostPort:      8084,
							ContainerPort: 84,
							Protocol:      v1.ProtocolTCP,
							HostIP:        "127.0.0.1",
						},
					},
				},
				expectError: false,
			},
			// same pod and portmappings,
			// but different IP must work
			{
				mapping: &PodPortMapping{
					Name:      "pod1",
					Namespace: "ns1",
					IP:        net.ParseIP("2001:beef::3"),
					PortMappings: []*PortMapping{
						{
							HostPort:      8080,
							ContainerPort: 80,
							Protocol:      v1.ProtocolTCP,
						},
						{
							HostPort:      8081,
							ContainerPort: 81,
							Protocol:      v1.ProtocolUDP,
						},
						{
							HostPort:      8083,
							ContainerPort: 83,
							Protocol:      v1.ProtocolSCTP,
						},
						{
							HostPort:      8084,
							ContainerPort: 84,
							Protocol:      v1.ProtocolTCP,
							HostIP:        "::1",
						},
					},
				},
				expectError: false,
			},
			{
				mapping: &PodPortMapping{
					Name:      "pod3",
					Namespace: "ns1",
					IP:        net.ParseIP("2001:beef::4"),
					PortMappings: []*PortMapping{
						{
							HostPort:      8443,
							ContainerPort: 443,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
				expectError: false,
			},
			// port already taken by other pod
			// but using another IP family
			{
				mapping: &PodPortMapping{
					Name:      "pod4",
					Namespace: "ns2",
					IP:        net.ParseIP("192.168.2.2"),
					PortMappings: []*PortMapping{
						{
							HostPort:      8443,
							ContainerPort: 443,
							Protocol:      v1.ProtocolTCP,
						},
					},
				},
				expectError: false,
			},
		}

		// Add Hostports
		for _, tc := range testCases {
			sandboxID := getPodFullName(tc.mapping)
			err := manager.Add(sandboxID, tc.mapping, "")
			if tc.expectError {
				Expect(err).To(HaveOccurred())
				continue
			}
			Expect(err).NotTo(HaveOccurred())
		}

		checkElements(nft4, []string{
			`add element ip crio-hostports hostports { tcp . 8080 comment "BBK5KOLM3RTTV4JS" : 192.168.2.7 . 80 }`,
			`add element ip crio-hostports hostports { udp . 8081 comment "BBK5KOLM3RTTV4JS" : 192.168.2.7 . 81 }`,
			`add element ip crio-hostports hostports { sctp . 8083 comment "BBK5KOLM3RTTV4JS" : 192.168.2.7 . 83 }`,
			`add element ip crio-hostports hostipports { 127.0.0.1 . tcp . 8084 comment "BBK5KOLM3RTTV4JS" : 192.168.2.7 . 84 }`,
			`add element ip crio-hostports hairpins { 192.168.2.7 . 192.168.2.7 comment "BBK5KOLM3RTTV4JS" }`,

			`add element ip crio-hostports hostports { tcp . 8443 comment "4TAICGIFOZBYBIFS" : 192.168.2.2 . 443 }`,
			`add element ip crio-hostports hairpins { 192.168.2.2 . 192.168.2.2 comment "4TAICGIFOZBYBIFS" }`,
		})

		// Check nftables after adding hostports
		checkElements(nft6, []string{
			`add element ip6 crio-hostports hostports { tcp . 8080 comment "BBK5KOLM3RTTV4JS" : 2001:beef::3 . 80 }`,
			`add element ip6 crio-hostports hostports { udp . 8081 comment "BBK5KOLM3RTTV4JS" : 2001:beef::3 . 81 }`,
			`add element ip6 crio-hostports hostports { sctp . 8083 comment "BBK5KOLM3RTTV4JS" : 2001:beef::3 . 83 }`,
			`add element ip6 crio-hostports hostipports { ::1 . tcp . 8084 comment "BBK5KOLM3RTTV4JS" : 2001:beef::3 . 84 }`,
			`add element ip6 crio-hostports hairpins { 2001:beef::3 . 2001:beef::3 comment "BBK5KOLM3RTTV4JS" }`,

			`add element ip6 crio-hostports hostports { tcp . 8443 comment "FWUCPWRIB7ZR62ZF" : 2001:beef::4 . 443 }`,
			`add element ip6 crio-hostports hairpins { 2001:beef::4 . 2001:beef::4 comment "FWUCPWRIB7ZR62ZF" }`,
		})

		// Remove all added hostports
		for _, tc := range testCases {
			if !tc.expectError {
				sandboxID := getPodFullName(tc.mapping)
				err := manager.Remove(sandboxID, tc.mapping)
				Expect(err).NotTo(HaveOccurred())
			}
		}

		// Check nftables after deleting hostports
		checkElements(nft4, []string{})
		checkElements(nft6, []string{})
	})
})

func checkElements(nft *knftables.Fake, expectedElements []string) {
	dump := nft.Dump()
	actualElements := make(map[string]bool)
	for _, line := range strings.Split(dump, "\n") {
		if strings.HasPrefix(line, "add element") {
			actualElements[line] = true
		}
	}
	for _, elem := range expectedElements {
		GinkgoWriter.Printf("Element: %s\n", elem)
		_, ok := actualElements[elem]
		ExpectWithOffset(1, ok).To(BeTrue(), "did not find %q in\n%s", elem, dump)
	}
	ExpectWithOffset(1, actualElements).To(HaveLen(len(expectedElements)), "wrong number of elements")
}
