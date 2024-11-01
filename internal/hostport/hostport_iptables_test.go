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
	"bytes"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"

	utiliptables "github.com/cri-o/cri-o/internal/iptables"
)

var expectedIPTablesRulesV4 = []string{
	"-A KUBE-HOSTPORTS -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp --dport 8443 -j KUBE-HP-WLTFZLTJ4QV7FRX3",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod3_ns1 hostport 8443\" -j CRIO-MASQ-WLTFZLTJ4QV7FRX3",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp --dport 8081 -j KUBE-HP-3MG73OVK5S7GSUBC",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8081\" -j CRIO-MASQ-3MG73OVK5S7GSUBC",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp --dport 8080 -j KUBE-HP-7BDNOFFT2YWI552I",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8080\" -j CRIO-MASQ-7BDNOFFT2YWI552I",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp --dport 8083 -j KUBE-HP-KYJTJFIY2JGKKVYU",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8083\" -j CRIO-MASQ-KYJTJFIY2JGKKVYU",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod5_ns5 hostport 8888\" -m tcp -p tcp --dport 8888 -j KUBE-HP-WTCIRE6PNE4I56DF",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8084\" -j CRIO-MASQ-IELLLSCZHX7UUOQQ",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8084\" -m tcp -p tcp --dport 8084 -j KUBE-HP-IELLLSCZHX7UUOQQ",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod5_ns5 hostport 8888\" -j CRIO-MASQ-WTCIRE6PNE4I56DF",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod5_ns5 hostport 8888\" -m tcp -p tcp --dport 8888 -j KUBE-HP-DQ5WDJN45DRPOYFE",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod5_ns5 hostport 8888\" -j CRIO-MASQ-DQ5WDJN45DRPOYFE",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod6_ns1 hostport 9999\" -m tcp -p tcp --dport 9999 -j KUBE-HP-AL32N6L3TM3M4FHI",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod6_ns1 hostport 9999\" -j CRIO-MASQ-AL32N6L3TM3M4FHI",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod6_ns1 hostport 9999\" -m udp -p udp --dport 9999 -j KUBE-HP-EOVTPYGVQGYVG7R5",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod6_ns1 hostport 9999\" -j CRIO-MASQ-EOVTPYGVQGYVG7R5",
	"-A CRIO-MASQ-7BDNOFFT2YWI552I -m comment --comment \"pod1_ns1 hostport 8080\" -m conntrack --ctorigdstport 8080 -m tcp -p tcp --dport 80 -s 10.1.1.2/32 -d 10.1.1.2/32 -j MASQUERADE",
	"-A KUBE-HP-7BDNOFFT2YWI552I -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp -j DNAT --to-destination 10.1.1.2:80",
	"-A CRIO-MASQ-3MG73OVK5S7GSUBC -m comment --comment \"pod1_ns1 hostport 8081\" -m conntrack --ctorigdstport 8081 -m udp -p udp --dport 81 -s 10.1.1.2/32 -d 10.1.1.2/32 -j MASQUERADE",
	"-A KUBE-HP-3MG73OVK5S7GSUBC -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp -j DNAT --to-destination 10.1.1.2:81",
	"-A CRIO-MASQ-KYJTJFIY2JGKKVYU -m comment --comment \"pod1_ns1 hostport 8083\" -m conntrack --ctorigdstport 8083 -m sctp -p sctp --dport 83 -s 10.1.1.2/32 -d 10.1.1.2/32 -j MASQUERADE",
	"-A KUBE-HP-KYJTJFIY2JGKKVYU -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp -j DNAT --to-destination 10.1.1.2:83",
	"-A CRIO-MASQ-IELLLSCZHX7UUOQQ -m comment --comment \"pod1_ns1 hostport 8084\" -m conntrack --ctorigdstport 8084 -m tcp -p tcp --dport 84 -s 10.1.1.2/32 -d 10.1.1.2/32 -j MASQUERADE",
	"-A KUBE-HP-IELLLSCZHX7UUOQQ -m comment --comment \"pod1_ns1 hostport 8084\" -m tcp -p tcp -d 127.0.0.1/32 -j DNAT --to-destination 10.1.1.2:84",
	"-A CRIO-MASQ-WLTFZLTJ4QV7FRX3 -m comment --comment \"pod3_ns1 hostport 8443\" -m conntrack --ctorigdstport 8443 -m tcp -p tcp --dport 443 -s 10.1.1.4/32 -d 10.1.1.4/32 -j MASQUERADE",
	"-A KUBE-HP-WLTFZLTJ4QV7FRX3 -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp -j DNAT --to-destination 10.1.1.4:443",
	"-A CRIO-MASQ-WTCIRE6PNE4I56DF -m comment --comment \"pod5_ns5 hostport 8888\" -m conntrack --ctorigdstport 8888 -m tcp -p tcp --dport 443 -s 10.1.1.5/32 -d 10.1.1.5/32 -j MASQUERADE",
	"-A KUBE-HP-WTCIRE6PNE4I56DF -m comment --comment \"pod5_ns5 hostport 8888\" -m tcp -p tcp -d 127.0.0.1/32 -j DNAT --to-destination 10.1.1.5:443",
	"-A CRIO-MASQ-DQ5WDJN45DRPOYFE -m comment --comment \"pod5_ns5 hostport 8888\" -m conntrack --ctorigdstport 8888 -m tcp -p tcp --dport 443 -s 10.1.1.5/32 -d 10.1.1.5/32 -j MASQUERADE",
	"-A KUBE-HP-DQ5WDJN45DRPOYFE -m comment --comment \"pod5_ns5 hostport 8888\" -m tcp -p tcp -d 127.0.0.2/32 -j DNAT --to-destination 10.1.1.5:443",
	"-A CRIO-MASQ-EOVTPYGVQGYVG7R5 -m comment --comment \"pod6_ns1 hostport 9999\" -m conntrack --ctorigdstport 9999 -m udp -p udp --dport 443 -s 10.1.1.6/32 -d 10.1.1.6/32 -j MASQUERADE",
	"-A KUBE-HP-AL32N6L3TM3M4FHI -m comment --comment \"pod6_ns1 hostport 9999\" -m tcp -p tcp -j DNAT --to-destination 10.1.1.6:443",
	"-A CRIO-MASQ-AL32N6L3TM3M4FHI -m comment --comment \"pod6_ns1 hostport 9999\" -m conntrack --ctorigdstport 9999 -m tcp -p tcp --dport 443 -s 10.1.1.6/32 -d 10.1.1.6/32 -j MASQUERADE",
	"-A KUBE-HP-EOVTPYGVQGYVG7R5 -m comment --comment \"pod6_ns1 hostport 9999\" -m udp -p udp -j DNAT --to-destination 10.1.1.6:443",
}

var expectedIPTablesRulesV6 = []string{
	"-A KUBE-HOSTPORTS -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp --dport 8443 -j KUBE-HP-WLTFZLTJ4QV7FRX3",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod3_ns1 hostport 8443\" -j CRIO-MASQ-WLTFZLTJ4QV7FRX3",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp --dport 8081 -j KUBE-HP-3MG73OVK5S7GSUBC",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8081\" -j CRIO-MASQ-3MG73OVK5S7GSUBC",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp --dport 8080 -j KUBE-HP-7BDNOFFT2YWI552I",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8080\" -j CRIO-MASQ-7BDNOFFT2YWI552I",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp --dport 8083 -j KUBE-HP-KYJTJFIY2JGKKVYU",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8083\" -j CRIO-MASQ-KYJTJFIY2JGKKVYU",
	"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8084\" -m tcp -p tcp --dport 8084 -j KUBE-HP-MGT4WGWISEW3X3JW",
	"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8084\" -j CRIO-MASQ-MGT4WGWISEW3X3JW",
	"-A CRIO-MASQ-7BDNOFFT2YWI552I -m comment --comment \"pod1_ns1 hostport 8080\" -m conntrack --ctorigdstport 8080 -m tcp -p tcp --dport 80 -s 2001:beef::2/128 -d 2001:beef::2/128 -j MASQUERADE",
	"-A KUBE-HP-7BDNOFFT2YWI552I -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp -j DNAT --to-destination [2001:beef::2]:80",
	"-A CRIO-MASQ-3MG73OVK5S7GSUBC -m comment --comment \"pod1_ns1 hostport 8081\" -m conntrack --ctorigdstport 8081 -m udp -p udp --dport 81 -s 2001:beef::2/128 -d 2001:beef::2/128 -j MASQUERADE",
	"-A KUBE-HP-3MG73OVK5S7GSUBC -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp -j DNAT --to-destination [2001:beef::2]:81",
	"-A CRIO-MASQ-KYJTJFIY2JGKKVYU -m comment --comment \"pod1_ns1 hostport 8083\" -m conntrack --ctorigdstport 8083 -m sctp -p sctp --dport 83 -s 2001:beef::2/128 -d 2001:beef::2/128 -j MASQUERADE",
	"-A KUBE-HP-KYJTJFIY2JGKKVYU -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp -j DNAT --to-destination [2001:beef::2]:83",
	"-A CRIO-MASQ-MGT4WGWISEW3X3JW -m comment --comment \"pod1_ns1 hostport 8084\" -m conntrack --ctorigdstport 8084 -m tcp -p tcp --dport 84 -s 2001:beef::2/128 -d 2001:beef::2/128 -j MASQUERADE",
	"-A KUBE-HP-MGT4WGWISEW3X3JW -m comment --comment \"pod1_ns1 hostport 8084\" -m tcp -p tcp -d ::1/128 -j DNAT --to-destination [2001:beef::2]:84",
	"-A CRIO-MASQ-WLTFZLTJ4QV7FRX3 -m comment --comment \"pod3_ns1 hostport 8443\" -m conntrack --ctorigdstport 8443 -m tcp -p tcp --dport 443 -s 2001:beef::4/128 -d 2001:beef::4/128 -j MASQUERADE",
	"-A KUBE-HP-WLTFZLTJ4QV7FRX3 -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp -j DNAT --to-destination [2001:beef::4]:443",
}

func checkIPTablesRules(ipt utiliptables.Interface, expectedRules []string) {
	raw := bytes.NewBuffer(nil)
	err := ipt.SaveInto(utiliptables.TableNAT, raw)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	expected := sets.New(expectedRules...)

	matched := sets.New[string]()

	for _, line := range strings.Split(raw.String(), "\n") {
		if strings.HasPrefix(line, "-A KUBE-HOSTPORTS ") || strings.HasPrefix(line, "-A CRIO-HOSTPORTS-MASQ ") || strings.HasPrefix(line, "-A KUBE-HP-") || strings.HasPrefix(line, "-A CRIO-MASQ-") {
			matched.Insert(line)
		}
	}

	unexpectedRules := matched.Difference(expected).UnsortedList()
	missingRules := expected.Difference(matched).UnsortedList()

	ExpectWithOffset(1, len(unexpectedRules)+len(missingRules)).To(Equal(0), "unexpected rules in iptables-save: %#v, expected rules missing from iptables-save: %#v", unexpectedRules, missingRules)
}

var _ = t.Describe("hostPortManagerIPTables", func() {
	It("should ensure kube hostport chains", func() {
		fakeIPTables := newFakeIPTables()
		Expect(ensureKubeHostportChains(fakeIPTables)).To(Succeed())

		_, _, err := fakeIPTables.getChain(utiliptables.TableNAT, utiliptables.Chain("KUBE-HOSTPORTS"))
		Expect(err).ToNot(HaveOccurred())

		builtinChains := []string{"PREROUTING", "OUTPUT"}
		hostPortJumpRule := "-m comment --comment \"kube hostport portals\" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS"

		for _, chainName := range builtinChains {
			_, chain, err := fakeIPTables.getChain(utiliptables.TableNAT, utiliptables.Chain(chainName))
			Expect(err).ToNot(HaveOccurred())
			Expect(len(chain.rules)).To(BeEquivalentTo(1))
			Expect(chain.rules).To(ContainElement(hostPortJumpRule))
		}

		masqJumpRule := "-m comment --comment \"kube hostport masquerading\" -m conntrack --ctstate DNAT -j CRIO-HOSTPORTS-MASQ"

		_, chain, err := fakeIPTables.getChain(utiliptables.TableNAT, utiliptables.ChainPostrouting)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(chain.rules)).To(BeEquivalentTo(1))
		Expect(chain.rules).To(ContainElement(masqJumpRule))

		_, chain, err = fakeIPTables.getChain(utiliptables.TableNAT, crioMasqueradeChain)
		Expect(err).ToNot(HaveOccurred())
		Expect(len(chain.rules)).To(BeEquivalentTo(0))
	})

	It("should create hostport chains with distinct names", func() {
		m := make(map[string]int)
		chain := getHostportChain("prefix", "testrdma-2", &PortMapping{HostPort: 57119, Protocol: "TCP", ContainerPort: 57119})
		m[string(chain)] = 1
		chain = getHostportChain("prefix", "testrdma-2", &PortMapping{HostPort: 55429, Protocol: "TCP", ContainerPort: 55429})
		m[string(chain)] = 1
		chain = getHostportChain("prefix", "testrdma-2", &PortMapping{HostPort: 56833, Protocol: "TCP", ContainerPort: 56833})
		m[string(chain)] = 1
		chain = getHostportChain("different-prefix", "testrdma-2", &PortMapping{HostPort: 56833, Protocol: "TCP", ContainerPort: 56833})
		m[string(chain)] = 1
		Expect(m).To(HaveLen(4))
	})

	It("should work for IPv4", func() {
		iptables := newFakeIPTables()
		iptables.protocol = utiliptables.ProtocolIPv4
		manager := &hostportManagerIPTables{
			iptables: iptables,
		}

		// Add Hostports
		for _, tc := range testCasesV4 {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after adding hostports
		checkIPTablesRules(manager.iptables, expectedIPTablesRulesV4)

		// Remove all added hostports
		for _, tc := range testCasesV4 {
			err := manager.Remove(tc.id, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after deleting hostports
		checkIPTablesRules(manager.iptables, nil)
	})

	It("should work for IPv6", func() {
		iptables := newFakeIPTables()
		iptables.protocol = utiliptables.ProtocolIPv6
		manager := &hostportManagerIPTables{
			iptables: iptables,
		}

		// Add Hostports
		for _, tc := range testCasesV6 {
			err := manager.Add(tc.id, tc.name, tc.podIP, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after adding hostports
		checkIPTablesRules(manager.iptables, expectedIPTablesRulesV6)

		// Remove all added hostports
		for _, tc := range testCasesV6 {
			err := manager.Remove(tc.id, tc.portMappings)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after deleting hostports
		checkIPTablesRules(manager.iptables, nil)
	})
})
