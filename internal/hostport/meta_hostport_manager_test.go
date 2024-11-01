package hostport

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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

	It("should succeed", func() {
		// ipv4
		iptables := newFakeIPTables()
		iptables.protocol = utiliptables.ProtocolIPv4
		// ipv6
		ip6tables := newFakeIPTables()
		ip6tables.protocol = utiliptables.ProtocolIPv6

		manager := metaHostportManager{
			ipv4HostportManager: &hostportManager{
				iptables: iptables,
			},
			ipv6HostportManager: &hostportManager{
				iptables: ip6tables,
			},
		}

		// Add Hostports
		for _, tc := range metaTestCases {
			err := manager.Add(tc.id, tc.mapping)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after adding hostports
		checkIPTablesRules(iptables, expectedRulesV4)
		checkIPTablesRules(ip6tables, expectedRulesV6)

		// Remove all added hostports
		for _, tc := range metaTestCases {
			err := manager.Remove(tc.id, tc.mapping)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after deleting hostports
		checkIPTablesRules(iptables, nil)
		checkIPTablesRules(ip6tables, nil)
	})
})
