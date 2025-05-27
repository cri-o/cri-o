package hostport

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	utilnet "k8s.io/utils/net"
	"sigs.k8s.io/knftables"

	utiliptables "github.com/cri-o/cri-o/internal/iptables"
)

var _ = t.Describe("MetaHostportManager", func() {
	if len(testCasesV4) < len(testCasesV6) {
		panic("internal error; expected more IPv4 than IPv6 test cases")
	}
	var metaTestCases []testCase
	for i := range testCasesV4 {
		metaTestCases = append(metaTestCases, testCasesV4[i])
		if i < len(testCasesV6) {
			metaTestCases = append(metaTestCases, testCasesV6[i])
		}
	}

	It("should work when only iptables is available", func() {
		iptables := newFakeIPTables()
		iptables.protocol = utiliptables.ProtocolIPv4
		ip6tables := newFakeIPTables()
		ip6tables.protocol = utiliptables.ProtocolIPv6

		manager := newMetaHostportManagerInternal(
			&hostportManagerIPTables{iptables: iptables},
			&hostportManagerIPTables{iptables: ip6tables},
			nil,
			nil,
		)

		// Add Hostports
		for _, tc := range metaTestCases {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after adding hostports
		checkIPTablesRules(iptables, expectedIPTablesRulesV4)
		checkIPTablesRules(ip6tables, expectedIPTablesRulesV6)

		// Remove all added hostports
		for _, tc := range metaTestCases {
			err := manager.Remove(tc.id, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after deleting hostports
		checkIPTablesRules(iptables, nil)
		checkIPTablesRules(ip6tables, nil)
	})

	It("should work when only nftables is available", func() {
		nft4 := knftables.NewFake(knftables.IPv4Family, hostPortsTable)
		nft6 := knftables.NewFake(knftables.IPv6Family, hostPortsTable)

		manager := newMetaHostportManagerInternal(
			nil,
			nil,
			&hostportManagerNFTables{nft: nft4, family: knftables.IPv4Family},
			&hostportManagerNFTables{nft: nft6, family: knftables.IPv6Family},
		)

		// Add Hostports
		for _, tc := range metaTestCases {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		checkNFTablesElements(nft4, expectedNFTablesElementsV4)
		checkNFTablesElements(nft6, expectedNFTablesElementsV6)

		// Remove all added hostports
		for _, tc := range metaTestCases {
			err := manager.Remove(tc.id, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		checkNFTablesElements(nft4, nil)
		checkNFTablesElements(nft6, nil)
	})

	It("should work when both iptables and nftables are available", func() {
		iptables := newFakeIPTables()
		iptables.protocol = utiliptables.ProtocolIPv4
		ip6tables := newFakeIPTables()
		ip6tables.protocol = utiliptables.ProtocolIPv6
		nft4 := knftables.NewFake(knftables.IPv4Family, hostPortsTable)
		nft6 := knftables.NewFake(knftables.IPv6Family, hostPortsTable)

		manager := newMetaHostportManagerInternal(
			&hostportManagerIPTables{iptables: iptables},
			&hostportManagerIPTables{iptables: ip6tables},
			&hostportManagerNFTables{nft: nft4, family: knftables.IPv4Family},
			&hostportManagerNFTables{nft: nft6, family: knftables.IPv6Family},
		)

		// Add Hostports
		for _, tc := range metaTestCases {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Should not have used iptables since nftables is present
		checkIPTablesRules(iptables, nil)
		checkIPTablesRules(ip6tables, nil)

		checkNFTablesElements(nft4, expectedNFTablesElementsV4)
		checkNFTablesElements(nft6, expectedNFTablesElementsV6)

		// Remove all added hostports
		for _, tc := range metaTestCases {
			err := manager.Remove(tc.id, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		checkIPTablesRules(iptables, nil)
		checkIPTablesRules(ip6tables, nil)
		checkNFTablesElements(nft4, nil)
		checkNFTablesElements(nft6, nil)
	})

	It("should clean up iptables when using nftables", func() {
		legacyIPTablesTestCases := []testCase{
			{
				id:    "8062968aa5c4d61f8963c53918e1edce3c86e9e7f63eddf941db339630ea985a",
				name:  "pod0_ns0",
				podIP: "10.1.1.1",
				portMappings: []*PortMapping{
					{
						HostPort:      9090,
						ContainerPort: 80,
						Protocol:      v1.ProtocolTCP,
					},
				},
			},
			{
				id:    "8062968aa5c4d61f8963c53918e1edce3c86e9e7f63eddf941db339630ea985a",
				name:  "pod0_ns0",
				podIP: "2001:beef::1",
				portMappings: []*PortMapping{
					{
						HostPort:      9090,
						ContainerPort: 80,
						Protocol:      v1.ProtocolTCP,
					},
				},
			},
		}
		legacyIPTablesExpectedRulesV4 := []string{
			"-A KUBE-HP-YJ3XFQDZHZ3NAUN2 -m comment --comment \"pod0_ns0 hostport 9090\" -m tcp -p tcp -j DNAT --to-destination 10.1.1.1:80",
			"-A CRIO-MASQ-YJ3XFQDZHZ3NAUN2 -m comment --comment \"pod0_ns0 hostport 9090\" -m conntrack --ctorigdstport 9090 -m tcp -p tcp --dport 80 -s 10.1.1.1/32 -d 10.1.1.1/32 -j MASQUERADE",
			"-A KUBE-HOSTPORTS -m comment --comment \"pod0_ns0 hostport 9090\" -m tcp -p tcp --dport 9090 -j KUBE-HP-YJ3XFQDZHZ3NAUN2",
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod0_ns0 hostport 9090\" -j CRIO-MASQ-YJ3XFQDZHZ3NAUN2",
		}
		legacyIPTablesExpectedRulesV6 := []string{
			"-A KUBE-HP-YJ3XFQDZHZ3NAUN2 -m comment --comment \"pod0_ns0 hostport 9090\" -m tcp -p tcp -j DNAT --to-destination [2001:beef::1]:80",
			"-A CRIO-MASQ-YJ3XFQDZHZ3NAUN2 -m comment --comment \"pod0_ns0 hostport 9090\" -m conntrack --ctorigdstport 9090 -m tcp -p tcp --dport 80 -s 2001:beef::1/128 -d 2001:beef::1/128 -j MASQUERADE",
			"-A KUBE-HOSTPORTS -m comment --comment \"pod0_ns0 hostport 9090\" -m tcp -p tcp --dport 9090 -j KUBE-HP-YJ3XFQDZHZ3NAUN2",
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod0_ns0 hostport 9090\" -j CRIO-MASQ-YJ3XFQDZHZ3NAUN2",
		}

		// Construct a metaHostportManager with only iptables support.
		iptables := newFakeIPTables()
		iptables.protocol = utiliptables.ProtocolIPv4
		ip6tables := newFakeIPTables()
		ip6tables.protocol = utiliptables.ProtocolIPv6

		manager := newMetaHostportManagerInternal(
			&hostportManagerIPTables{iptables: iptables},
			&hostportManagerIPTables{iptables: ip6tables},
			nil,
			nil,
		)

		// Add the legacy mappings.
		for _, tc := range legacyIPTablesTestCases {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		checkIPTablesRules(iptables, legacyIPTablesExpectedRulesV4)
		checkIPTablesRules(ip6tables, legacyIPTablesExpectedRulesV6)

		// "Quit and restart cri-o", and create a new metaHostportManager with the
		// existing fakeIPTables state, but now with nftables support as well.
		nft4 := knftables.NewFake(knftables.IPv4Family, hostPortsTable)
		nft6 := knftables.NewFake(knftables.IPv6Family, hostPortsTable)

		manager = newMetaHostportManagerInternal(
			&hostportManagerIPTables{iptables: iptables},
			&hostportManagerIPTables{iptables: ip6tables},
			&hostportManagerNFTables{nft: nft4, family: knftables.IPv4Family},
			&hostportManagerNFTables{nft: nft6, family: knftables.IPv6Family},
		)

		// Add the remaining hostports.
		for _, tc := range metaTestCases {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// iptables mappings should not have changed; we should have added the new
		// mappings to nftables.
		checkIPTablesRules(iptables, legacyIPTablesExpectedRulesV4)
		checkIPTablesRules(ip6tables, legacyIPTablesExpectedRulesV6)

		checkNFTablesElements(nft4, expectedNFTablesElementsV4)
		checkNFTablesElements(nft6, expectedNFTablesElementsV6)

		// Remove all added hostports
		for _, tc := range legacyIPTablesTestCases {
			err := manager.Remove(tc.id, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}
		for _, tc := range metaTestCases {
			err := manager.Remove(tc.id, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// iptables and nftables should both have been removed.
		checkIPTablesRules(iptables, nil)
		checkIPTablesRules(ip6tables, nil)
		checkNFTablesElements(nft4, nil)
		checkNFTablesElements(nft6, nil)
	})

	It("should work with just IPv4", func() {
		iptables := newFakeIPTables()
		iptables.protocol = utiliptables.ProtocolIPv4
		nft4 := knftables.NewFake(knftables.IPv4Family, hostPortsTable)

		manager := newMetaHostportManagerInternal(
			&hostportManagerIPTables{iptables: iptables},
			nil,
			&hostportManagerNFTables{nft: nft4, family: knftables.IPv4Family},
			nil,
		)

		// Add Hostports
		for _, tc := range metaTestCases {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			// IPv6 mappings should fail because we don't have any IPv6
			// HostPortManagers.
			if utilnet.IsIPv6String(tc.podIP) {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		}

		checkIPTablesRules(iptables, nil)
		checkNFTablesElements(nft4, expectedNFTablesElementsV4)

		// Remove all added hostports
		for _, tc := range metaTestCases {
			err := manager.Remove(tc.id, tc.portMappings)
			// Remove is IP-family agnostic and should just ignore the missing
			// IPv6 HostPortManagers.
			Expect(err).NotTo(HaveOccurred())
		}

		checkIPTablesRules(iptables, nil)
		checkNFTablesElements(nft4, nil)
	})
})
