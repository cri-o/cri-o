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
	"net"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"

	utiliptables "github.com/cri-o/cri-o/internal/iptables"
)

// newFakeManager creates a new Manager with fake iptables. Note that we need to create
// (and semi-initialize) both ip4tables and ip6tables even for the single-stack tests,
// because Remove() will try to use both.
func newFakeManager() *hostportManager {
	ip4tables := newFakeIPTables()
	ip4tables.protocol = utiliptables.ProtocolIPv4
	//nolint:errcheck // can't fail with fake iptables
	_, _ = ip4tables.EnsureChain(utiliptables.TableNAT, utiliptables.ChainOutput)

	ip6tables := newFakeIPTables()
	ip6tables.protocol = utiliptables.ProtocolIPv6
	//nolint:errcheck // can't fail with fake iptables
	_, _ = ip6tables.EnsureChain(utiliptables.TableNAT, utiliptables.ChainOutput)

	return &hostportManager{
		ip4tables: ip4tables,
		ip6tables: ip6tables,
	}
}

var _ = t.Describe("HostPortManager", func() {
	It("HostportManagerIPv4", func() {
		manager := newFakeManager()
		testCases := []struct {
			mapping *PodPortMapping
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
			},
			// open same HostPort on different
			{
				mapping: &PodPortMapping{
					Name:      "pod6",
					Namespace: "ns1",
					IP:        net.ParseIP("10.1.1.2"),
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
			},
		}

		// Add Hostports
		for _, tc := range testCases {
			err := manager.Add("id", tc.mapping)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after adding hostports
		raw := bytes.NewBuffer(nil)
		err := manager.ip4tables.SaveInto(utiliptables.TableNAT, raw)
		Expect(err).NotTo(HaveOccurred())

		lines := strings.Split(raw.String(), "\n")
		expectedLines := map[string]bool{
			`*nat`:                                true,
			`:KUBE-HOSTPORTS - [0:0]`:             true,
			`:CRIO-HOSTPORTS-MASQ - [0:0]`:        true,
			`:OUTPUT - [0:0]`:                     true,
			`:PREROUTING - [0:0]`:                 true,
			`:POSTROUTING - [0:0]`:                true,
			`:KUBE-HP-IJHALPHTORMHHPPK - [0:0]`:   true,
			`:CRIO-MASQ-IJHALPHTORMHHPPK - [0:0]`: true,
			`:KUBE-HP-63UPIDJXVRSZGSUZ - [0:0]`:   true,
			`:CRIO-MASQ-63UPIDJXVRSZGSUZ - [0:0]`: true,
			`:KUBE-HP-WFBOALXEP42XEMJK - [0:0]`:   true,
			`:CRIO-MASQ-WFBOALXEP42XEMJK - [0:0]`: true,
			`:KUBE-HP-XU6AWMMJYOZOFTFZ - [0:0]`:   true,
			`:CRIO-MASQ-XU6AWMMJYOZOFTFZ - [0:0]`: true,
			`:KUBE-HP-TUKTZ736U5JD5UTK - [0:0]`:   true,
			`:CRIO-MASQ-TUKTZ736U5JD5UTK - [0:0]`: true,
			`:KUBE-HP-CAAJ45HDITK7ARGM - [0:0]`:   true,
			`:CRIO-MASQ-CAAJ45HDITK7ARGM - [0:0]`: true,
			`:KUBE-HP-WFUNFVXVDLD5ZVXN - [0:0]`:   true,
			`:CRIO-MASQ-WFUNFVXVDLD5ZVXN - [0:0]`: true,
			`:KUBE-HP-4MFWH2F2NAOMYD6A - [0:0]`:   true,
			`:CRIO-MASQ-4MFWH2F2NAOMYD6A - [0:0]`: true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp --dport 8443 -j KUBE-HP-WFBOALXEP42XEMJK":                                                               true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod3_ns1 hostport 8443\" -j CRIO-MASQ-WFBOALXEP42XEMJK":                                                                                   true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp --dport 8081 -j KUBE-HP-63UPIDJXVRSZGSUZ":                                                               true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8081\" -j CRIO-MASQ-63UPIDJXVRSZGSUZ":                                                                                   true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp --dport 8080 -j KUBE-HP-IJHALPHTORMHHPPK":                                                               true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8080\" -j CRIO-MASQ-IJHALPHTORMHHPPK":                                                                                   true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp --dport 8083 -j KUBE-HP-XU6AWMMJYOZOFTFZ":                                                             true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8083\" -j CRIO-MASQ-XU6AWMMJYOZOFTFZ":                                                                                   true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod5_ns5 hostport 8888\" -m tcp -p tcp --dport 8888 -j KUBE-HP-TUKTZ736U5JD5UTK":                                                               true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod5_ns5 hostport 8888\" -j CRIO-MASQ-TUKTZ736U5JD5UTK":                                                                                   true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod5_ns5 hostport 8888\" -m tcp -p tcp --dport 8888 -j KUBE-HP-CAAJ45HDITK7ARGM":                                                               true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod5_ns5 hostport 8888\" -j CRIO-MASQ-CAAJ45HDITK7ARGM":                                                                                   true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod6_ns1 hostport 9999\" -m udp -p udp --dport 9999 -j KUBE-HP-4MFWH2F2NAOMYD6A":                                                               true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod6_ns1 hostport 9999\" -j CRIO-MASQ-4MFWH2F2NAOMYD6A":                                                                                   true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod6_ns1 hostport 9999\" -m tcp -p tcp --dport 9999 -j KUBE-HP-WFUNFVXVDLD5ZVXN":                                                               true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod6_ns1 hostport 9999\" -j CRIO-MASQ-WFUNFVXVDLD5ZVXN":                                                                                   true,
			"-A OUTPUT -m comment --comment \"kube hostport portals\" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS":                                                                                true,
			"-A PREROUTING -m comment --comment \"kube hostport portals\" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS":                                                                            true,
			"-A POSTROUTING -m comment --comment \"kube hostport masquerading\" -m conntrack --ctstate DNAT -j CRIO-HOSTPORTS-MASQ":                                                                  true,
			"-A CRIO-MASQ-IJHALPHTORMHHPPK -m comment --comment \"pod1_ns1 hostport 8080\" -m conntrack --ctorigdstport 8080 -m tcp -p tcp --dport 80 -s 10.1.1.2/32 -d 10.1.1.2/32 -j MASQUERADE":   true,
			"-A KUBE-HP-IJHALPHTORMHHPPK -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp -j DNAT --to-destination 10.1.1.2:80":                                                         true,
			"-A CRIO-MASQ-63UPIDJXVRSZGSUZ -m comment --comment \"pod1_ns1 hostport 8081\" -m conntrack --ctorigdstport 8081 -m udp -p udp --dport 81 -s 10.1.1.2/32 -d 10.1.1.2/32 -j MASQUERADE":   true,
			"-A KUBE-HP-63UPIDJXVRSZGSUZ -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp -j DNAT --to-destination 10.1.1.2:81":                                                         true,
			"-A CRIO-MASQ-XU6AWMMJYOZOFTFZ -m comment --comment \"pod1_ns1 hostport 8083\" -m conntrack --ctorigdstport 8083 -m sctp -p sctp --dport 83 -s 10.1.1.2/32 -d 10.1.1.2/32 -j MASQUERADE": true,
			"-A KUBE-HP-XU6AWMMJYOZOFTFZ -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp -j DNAT --to-destination 10.1.1.2:83":                                                       true,
			"-A CRIO-MASQ-WFBOALXEP42XEMJK -m comment --comment \"pod3_ns1 hostport 8443\" -m conntrack --ctorigdstport 8443 -m tcp -p tcp --dport 443 -s 10.1.1.4/32 -d 10.1.1.4/32 -j MASQUERADE":  true,
			"-A KUBE-HP-WFBOALXEP42XEMJK -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp -j DNAT --to-destination 10.1.1.4:443":                                                        true,
			"-A CRIO-MASQ-TUKTZ736U5JD5UTK -m comment --comment \"pod5_ns5 hostport 8888\" -m conntrack --ctorigdstport 8888 -m tcp -p tcp --dport 443 -s 10.1.1.5/32 -d 10.1.1.5/32 -j MASQUERADE":  true,
			"-A KUBE-HP-TUKTZ736U5JD5UTK -m comment --comment \"pod5_ns5 hostport 8888\" -m tcp -p tcp -d 127.0.0.1/32 -j DNAT --to-destination 10.1.1.5:443":                                        true,
			"-A CRIO-MASQ-CAAJ45HDITK7ARGM -m comment --comment \"pod5_ns5 hostport 8888\" -m conntrack --ctorigdstport 8888 -m tcp -p tcp --dport 443 -s 10.1.1.5/32 -d 10.1.1.5/32 -j MASQUERADE":  true,
			"-A KUBE-HP-CAAJ45HDITK7ARGM -m comment --comment \"pod5_ns5 hostport 8888\" -m tcp -p tcp -d 127.0.0.2/32 -j DNAT --to-destination 10.1.1.5:443":                                        true,
			"-A CRIO-MASQ-WFUNFVXVDLD5ZVXN -m comment --comment \"pod6_ns1 hostport 9999\" -m conntrack --ctorigdstport 9999 -m tcp -p tcp --dport 443 -s 10.1.1.2/32 -d 10.1.1.2/32 -j MASQUERADE":  true,
			"-A KUBE-HP-WFUNFVXVDLD5ZVXN -m comment --comment \"pod6_ns1 hostport 9999\" -m tcp -p tcp -j DNAT --to-destination 10.1.1.2:443":                                                        true,
			"-A CRIO-MASQ-4MFWH2F2NAOMYD6A -m comment --comment \"pod6_ns1 hostport 9999\" -m conntrack --ctorigdstport 9999 -m udp -p udp --dport 443 -s 10.1.1.2/32 -d 10.1.1.2/32 -j MASQUERADE":  true,
			"-A KUBE-HP-4MFWH2F2NAOMYD6A -m comment --comment \"pod6_ns1 hostport 9999\" -m udp -p udp -j DNAT --to-destination 10.1.1.2:443":                                                        true,
			`COMMIT`: true,
		}
		for _, line := range lines {
			GinkgoWriter.Printf("Line: %s\n", line)
			if strings.TrimSpace(line) != "" {
				_, ok := expectedLines[strings.TrimSpace(line)]
				Expect(ok).To(BeTrue())
			}
		}

		// Remove all added hostports
		for _, tc := range testCases {
			err := manager.Remove("id", tc.mapping)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after deleting hostports
		raw.Reset()
		err = manager.ip4tables.SaveInto(utiliptables.TableNAT, raw)
		Expect(err).NotTo(HaveOccurred())
		lines = strings.Split(raw.String(), "\n")
		remainingChains := make(map[string]bool)
		for _, line := range lines {
			if strings.HasPrefix(line, ":") {
				remainingChains[strings.TrimSpace(line)] = true
			}
		}
		expectDeletedChains := []string{
			"KUBE-HP-4YVONL46AKYWSKS3", "KUBE-HP-7THKRFSEH4GIIXK7", "KUBE-HP-5N7UH5JAXCVP5UJR",
			"KUBE-HP-TUKTZ736U5JD5UTK", "KUBE-HP-CAAJ45HDITK7ARGM", "KUBE-HP-WFUNFVXVDLD5ZVXN", "KUBE-HP-4MFWH2F2NAOMYD6A",
		}
		for _, chain := range expectDeletedChains {
			_, ok := remainingChains[chain]
			Expect(ok).To(BeFalse())
		}
	})

	It("GetHostportChain", func() {
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

	It("HostportManagerIPv6", func() {
		manager := newFakeManager()
		testCases := []struct {
			mapping *PodPortMapping
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
			},
		}

		// Add Hostports
		for _, tc := range testCases {
			err := manager.Add("id", tc.mapping)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after adding hostports
		raw := bytes.NewBuffer(nil)
		err := manager.ip6tables.SaveInto(utiliptables.TableNAT, raw)
		Expect(err).NotTo(HaveOccurred())

		lines := strings.Split(raw.String(), "\n")
		expectedLines := map[string]bool{
			`*nat`:                                true,
			`:KUBE-HOSTPORTS - [0:0]`:             true,
			`:CRIO-HOSTPORTS-MASQ - [0:0]`:        true,
			`:OUTPUT - [0:0]`:                     true,
			`:PREROUTING - [0:0]`:                 true,
			`:POSTROUTING - [0:0]`:                true,
			`:KUBE-HP-IJHALPHTORMHHPPK - [0:0]`:   true,
			`:CRIO-MASQ-IJHALPHTORMHHPPK - [0:0]`: true,
			`:KUBE-HP-63UPIDJXVRSZGSUZ - [0:0]`:   true,
			`:CRIO-MASQ-63UPIDJXVRSZGSUZ - [0:0]`: true,
			`:KUBE-HP-WFBOALXEP42XEMJK - [0:0]`:   true,
			`:CRIO-MASQ-WFBOALXEP42XEMJK - [0:0]`: true,
			`:KUBE-HP-XU6AWMMJYOZOFTFZ - [0:0]`:   true,
			`:CRIO-MASQ-XU6AWMMJYOZOFTFZ - [0:0]`: true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp --dport 8443 -j KUBE-HP-WFBOALXEP42XEMJK":                                                                       true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod3_ns1 hostport 8443\" -j CRIO-MASQ-WFBOALXEP42XEMJK":                                                                                           true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp --dport 8081 -j KUBE-HP-63UPIDJXVRSZGSUZ":                                                                       true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8081\" -j CRIO-MASQ-63UPIDJXVRSZGSUZ":                                                                                           true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp --dport 8080 -j KUBE-HP-IJHALPHTORMHHPPK":                                                                       true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8080\" -j CRIO-MASQ-IJHALPHTORMHHPPK":                                                                                           true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp --dport 8083 -j KUBE-HP-XU6AWMMJYOZOFTFZ":                                                                     true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8083\" -j CRIO-MASQ-XU6AWMMJYOZOFTFZ":                                                                                           true,
			"-A OUTPUT -m comment --comment \"kube hostport portals\" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS":                                                                                        true,
			"-A PREROUTING -m comment --comment \"kube hostport portals\" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS":                                                                                    true,
			"-A POSTROUTING -m comment --comment \"kube hostport masquerading\" -m conntrack --ctstate DNAT -j CRIO-HOSTPORTS-MASQ":                                                                          true,
			"-A CRIO-MASQ-IJHALPHTORMHHPPK -m comment --comment \"pod1_ns1 hostport 8080\" -m conntrack --ctorigdstport 8080 -m tcp -p tcp --dport 80 -s 2001:beef::2/32 -d 2001:beef::2/32 -j MASQUERADE":   true,
			"-A KUBE-HP-IJHALPHTORMHHPPK -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp -j DNAT --to-destination [2001:beef::2]:80":                                                           true,
			"-A CRIO-MASQ-63UPIDJXVRSZGSUZ -m comment --comment \"pod1_ns1 hostport 8081\" -m conntrack --ctorigdstport 8081 -m udp -p udp --dport 81 -s 2001:beef::2/32 -d 2001:beef::2/32 -j MASQUERADE":   true,
			"-A KUBE-HP-63UPIDJXVRSZGSUZ -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp -j DNAT --to-destination [2001:beef::2]:81":                                                           true,
			"-A CRIO-MASQ-XU6AWMMJYOZOFTFZ -m comment --comment \"pod1_ns1 hostport 8083\" -m conntrack --ctorigdstport 8083 -m sctp -p sctp --dport 83 -s 2001:beef::2/32 -d 2001:beef::2/32 -j MASQUERADE": true,
			"-A KUBE-HP-XU6AWMMJYOZOFTFZ -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp -j DNAT --to-destination [2001:beef::2]:83":                                                         true,
			"-A CRIO-MASQ-WFBOALXEP42XEMJK -m comment --comment \"pod3_ns1 hostport 8443\" -m conntrack --ctorigdstport 8443 -m tcp -p tcp --dport 443 -s 2001:beef::4/32 -d 2001:beef::4/32 -j MASQUERADE":  true,
			"-A KUBE-HP-WFBOALXEP42XEMJK -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp -j DNAT --to-destination [2001:beef::4]:443":                                                          true,
			`COMMIT`: true,
		}
		for _, line := range lines {
			GinkgoWriter.Printf("Line: %s\n", line)
			if strings.TrimSpace(line) != "" {
				_, ok := expectedLines[strings.TrimSpace(line)]
				Expect(ok).To(BeTrue())
			}
		}

		// Remove all added hostports
		for _, tc := range testCases {
			err := manager.Remove("id", tc.mapping)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check Iptables-save result after deleting hostports
		raw.Reset()
		err = manager.ip6tables.SaveInto(utiliptables.TableNAT, raw)
		Expect(err).NotTo(HaveOccurred())
		lines = strings.Split(raw.String(), "\n")
		remainingChains := make(map[string]bool)
		for _, line := range lines {
			if strings.HasPrefix(line, ":") {
				remainingChains[strings.TrimSpace(line)] = true
			}
		}
		expectDeletedChains := []string{"KUBE-HP-4YVONL46AKYWSKS3", "KUBE-HP-7THKRFSEH4GIIXK7", "KUBE-HP-5N7UH5JAXCVP5UJR"}
		for _, chain := range expectDeletedChains {
			_, ok := remainingChains[chain]
			Expect(ok).To(BeFalse())
		}
	})

	It("HostportManagerDualStack", func() {
		manager := newFakeManager()
		testCases := []struct {
			mapping *PodPortMapping
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
							HostIP:        "127.0.0.1",
						},
					},
				},
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
			},
		}

		// Add Hostports
		for _, tc := range testCases {
			err := manager.Add("id", tc.mapping)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check IPv4 Iptables-save result after adding hostports
		raw := bytes.NewBuffer(nil)

		err := manager.ip4tables.SaveInto(utiliptables.TableNAT, raw)
		Expect(err).NotTo(HaveOccurred())

		lines := strings.Split(raw.String(), "\n")
		expectedLines := map[string]bool{
			`*nat`:                                true,
			`:KUBE-HOSTPORTS - [0:0]`:             true,
			`:CRIO-HOSTPORTS-MASQ - [0:0]`:        true,
			`:OUTPUT - [0:0]`:                     true,
			`:PREROUTING - [0:0]`:                 true,
			`:POSTROUTING - [0:0]`:                true,
			`:KUBE-HP-IJHALPHTORMHHPPK - [0:0]`:   true,
			`:CRIO-MASQ-IJHALPHTORMHHPPK - [0:0]`: true,
			`:KUBE-HP-63UPIDJXVRSZGSUZ - [0:0]`:   true,
			`:CRIO-MASQ-63UPIDJXVRSZGSUZ - [0:0]`: true,
			`:KUBE-HP-WFBOALXEP42XEMJK - [0:0]`:   true,
			`:CRIO-MASQ-WFBOALXEP42XEMJK - [0:0]`: true,
			`:KUBE-HP-XU6AWMMJYOZOFTFZ - [0:0]`:   true,
			`:CRIO-MASQ-XU6AWMMJYOZOFTFZ - [0:0]`: true,
			`:KUBE-HP-CHN66X54O4WXZ5CW - [0:0]`:   true,
			`:CRIO-MASQ-CHN66X54O4WXZ5CW - [0:0]`: true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp --dport 8081 -j KUBE-HP-63UPIDJXVRSZGSUZ":                                                                     true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8081\" -j CRIO-MASQ-63UPIDJXVRSZGSUZ":                                                                                         true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp --dport 8080 -j KUBE-HP-IJHALPHTORMHHPPK":                                                                     true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8080\" -j CRIO-MASQ-IJHALPHTORMHHPPK":                                                                                         true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp --dport 8083 -j KUBE-HP-XU6AWMMJYOZOFTFZ":                                                                   true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8083\" -j CRIO-MASQ-XU6AWMMJYOZOFTFZ":                                                                                         true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8084\" -m tcp -p tcp --dport 8084 -j KUBE-HP-CHN66X54O4WXZ5CW":                                                                     true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8084\" -j CRIO-MASQ-CHN66X54O4WXZ5CW":                                                                                         true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod4_ns2 hostport 8443\" -m tcp -p tcp --dport 8443 -j KUBE-HP-WFBOALXEP42XEMJK":                                                                     true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod4_ns2 hostport 8443\" -j CRIO-MASQ-WFBOALXEP42XEMJK":                                                                                         true,
			"-A OUTPUT -m comment --comment \"kube hostport portals\" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS":                                                                                      true,
			"-A PREROUTING -m comment --comment \"kube hostport portals\" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS":                                                                                  true,
			"-A POSTROUTING -m comment --comment \"kube hostport masquerading\" -m conntrack --ctstate DNAT -j CRIO-HOSTPORTS-MASQ":                                                                        true,
			"-A CRIO-MASQ-IJHALPHTORMHHPPK -m comment --comment \"pod1_ns1 hostport 8080\" -m conntrack --ctorigdstport 8080 -m tcp -p tcp --dport 80 -s 192.168.2.7/32 -d 192.168.2.7/32 -j MASQUERADE":   true,
			"-A KUBE-HP-IJHALPHTORMHHPPK -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp -j DNAT --to-destination 192.168.2.7:80":                                                            true,
			"-A CRIO-MASQ-63UPIDJXVRSZGSUZ -m comment --comment \"pod1_ns1 hostport 8081\" -m conntrack --ctorigdstport 8081 -m udp -p udp --dport 81 -s 192.168.2.7/32 -d 192.168.2.7/32 -j MASQUERADE":   true,
			"-A KUBE-HP-63UPIDJXVRSZGSUZ -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp -j DNAT --to-destination 192.168.2.7:81":                                                            true,
			"-A CRIO-MASQ-XU6AWMMJYOZOFTFZ -m comment --comment \"pod1_ns1 hostport 8083\" -m conntrack --ctorigdstport 8083 -m sctp -p sctp --dport 83 -s 192.168.2.7/32 -d 192.168.2.7/32 -j MASQUERADE": true,
			"-A KUBE-HP-XU6AWMMJYOZOFTFZ -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp -j DNAT --to-destination 192.168.2.7:83":                                                          true,
			"-A CRIO-MASQ-CHN66X54O4WXZ5CW -m comment --comment \"pod1_ns1 hostport 8084\" -m conntrack --ctorigdstport 8084 -m tcp -p tcp --dport 84 -s 192.168.2.7/32 -d 192.168.2.7/32 -j MASQUERADE":   true,
			"-A KUBE-HP-CHN66X54O4WXZ5CW -m comment --comment \"pod1_ns1 hostport 8084\" -m tcp -p tcp -d 127.0.0.1/32 -j DNAT --to-destination 192.168.2.7:84":                                            true,
			"-A CRIO-MASQ-WFBOALXEP42XEMJK -m comment --comment \"pod4_ns2 hostport 8443\" -m conntrack --ctorigdstport 8443 -m tcp -p tcp --dport 443 -s 192.168.2.2/32 -d 192.168.2.2/32 -j MASQUERADE":  true,
			"-A KUBE-HP-WFBOALXEP42XEMJK -m comment --comment \"pod4_ns2 hostport 8443\" -m tcp -p tcp -j DNAT --to-destination 192.168.2.2:443":                                                           true,
			`COMMIT`: true,
		}
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				_, ok := expectedLines[strings.TrimSpace(line)]
				Expect(ok).To(BeTrue())
			}
		}

		// Remove all added hostports
		for _, tc := range testCases {
			err := manager.Remove("id", tc.mapping)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check IPv4 Iptables-save result after deleting hostports
		raw.Reset()
		err = manager.ip4tables.SaveInto(utiliptables.TableNAT, raw)
		Expect(err).NotTo(HaveOccurred())
		lines = strings.Split(raw.String(), "\n")
		remainingChains := make(map[string]bool)
		for _, line := range lines {
			if strings.HasPrefix(line, ":") {
				remainingChains[strings.TrimSpace(line)] = true
			}
		}
		expectDeletedChains := []string{"KUBE-HP-4YVONL46AKYWSKS3", "KUBE-HP-7THKRFSEH4GIIXK7", "KUBE-HP-5N7UH5JAXCVP5UJR", "KUBE-HP-CHN66X54O4WXZ5CW"}
		for _, chain := range expectDeletedChains {
			_, ok := remainingChains[chain]
			Expect(ok).To(BeFalse())
		}

		// Check IPv6 Iptables-save result after adding hostports
		rawv6 := bytes.NewBuffer(nil)

		err = manager.ip6tables.SaveInto(utiliptables.TableNAT, rawv6)
		Expect(err).NotTo(HaveOccurred())

		linesv6 := strings.Split(rawv6.String(), "\n")
		expectedv6Lines := map[string]bool{
			`*nat`:                                true,
			`:KUBE-HOSTPORTS - [0:0]`:             true,
			`:CRIO-HOSTPORTS-MASQ - [0:0]`:        true,
			`:OUTPUT - [0:0]`:                     true,
			`:PREROUTING - [0:0]`:                 true,
			`:POSTROUTING - [0:0]`:                true,
			`:KUBE-HP-IJHALPHTORMHHPPK - [0:0]`:   true,
			`:CRIO-MASQ-IJHALPHTORMHHPPK - [0:0]`: true,
			`:KUBE-HP-63UPIDJXVRSZGSUZ - [0:0]`:   true,
			`:CRIO-MASQ-63UPIDJXVRSZGSUZ - [0:0]`: true,
			`:KUBE-HP-WFBOALXEP42XEMJK - [0:0]`:   true,
			`:CRIO-MASQ-WFBOALXEP42XEMJK - [0:0]`: true,
			`:KUBE-HP-XU6AWMMJYOZOFTFZ - [0:0]`:   true,
			`:CRIO-MASQ-XU6AWMMJYOZOFTFZ - [0:0]`: true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp --dport 8443 -j KUBE-HP-WFBOALXEP42XEMJK":                                                                      true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod3_ns1 hostport 8443\" -j CRIO-MASQ-WFBOALXEP42XEMJK":                                                                                          true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp --dport 8081 -j KUBE-HP-63UPIDJXVRSZGSUZ":                                                                      true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8081\" -j CRIO-MASQ-63UPIDJXVRSZGSUZ":                                                                                          true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp --dport 8080 -j KUBE-HP-IJHALPHTORMHHPPK":                                                                      true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8080\" -j CRIO-MASQ-IJHALPHTORMHHPPK":                                                                                          true,
			"-A KUBE-HOSTPORTS -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp --dport 8083 -j KUBE-HP-XU6AWMMJYOZOFTFZ":                                                                    true,
			"-A CRIO-HOSTPORTS-MASQ -m comment --comment \"pod1_ns1 hostport 8083\" -j CRIO-MASQ-XU6AWMMJYOZOFTFZ":                                                                                          true,
			"-A OUTPUT -m comment --comment \"kube hostport portals\" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS":                                                                                       true,
			"-A PREROUTING -m comment --comment \"kube hostport portals\" -m addrtype --dst-type LOCAL -j KUBE-HOSTPORTS":                                                                                   true,
			"-A POSTROUTING -m comment --comment \"kube hostport masquerading\" -m conntrack --ctstate DNAT -j CRIO-HOSTPORTS-MASQ":                                                                         true,
			"-A CRIO-MASQ-IJHALPHTORMHHPPK -m comment --comment \"pod1_ns1 hostport 8080\" -m conntrack --ctorigdstport 9999 -m tcp -p tcp --dport 443 -s 2001:beef::2/32 -d 2001:beef::2/32 -j MASQUERADE": true,
			"-A KUBE-HP-IJHALPHTORMHHPPK -m comment --comment \"pod1_ns1 hostport 8080\" -m tcp -p tcp -j DNAT --to-destination [2001:beef::2]:80":                                                          true,
			"-A CRIO-MASQ-63UPIDJXVRSZGSUZ -m comment --comment \"pod1_ns1 hostport 8081\" -m conntrack --ctorigdstport 9999 -m tcp -p tcp --dport 443 -s 2001:beef::2/32 -d 2001:beef::2/32 -j MASQUERADE": true,
			"-A KUBE-HP-63UPIDJXVRSZGSUZ -m comment --comment \"pod1_ns1 hostport 8081\" -m udp -p udp -j DNAT --to-destination [2001:beef::2]:81":                                                          true,
			"-A CRIO-MASQ-XU6AWMMJYOZOFTFZ -m comment --comment \"pod1_ns1 hostport 8083\" -m conntrack --ctorigdstport 9999 -m tcp -p tcp --dport 443 -s 2001:beef::2/32 -d 2001:beef::2/32 -j MASQUERADE": true,
			"-A KUBE-HP-XU6AWMMJYOZOFTFZ -m comment --comment \"pod1_ns1 hostport 8083\" -m sctp -p sctp -j DNAT --to-destination [2001:beef::2]:83":                                                        true,
			"-A CRIO-MASQ-WFBOALXEP42XEMJK -m comment --comment \"pod3_ns1 hostport 8443\" -m conntrack --ctorigdstport 9999 -m tcp -p tcp --dport 443 -s 2001:beef::4/32 -d 2001:beef::4/32 -j MASQUERADE": true,
			"-A KUBE-HP-WFBOALXEP42XEMJK -m comment --comment \"pod3_ns1 hostport 8443\" -m tcp -p tcp -j DNAT --to-destination [2001:beef::4]:443":                                                         true,
			`COMMIT`: true,
		}
		for _, line := range linesv6 {
			if strings.TrimSpace(line) != "" {
				_, ok := expectedv6Lines[strings.TrimSpace(line)]
				Expect(ok).To(BeTrue())
			}
		}

		// Remove all added hostports
		for _, tc := range testCases {
			err := manager.Remove("id", tc.mapping)
			Expect(err).NotTo(HaveOccurred())
		}

		// Check IPv6 Iptables-save result after deleting hostports
		rawv6.Reset()
		err = manager.ip6tables.SaveInto(utiliptables.TableNAT, rawv6)
		Expect(err).NotTo(HaveOccurred())
		linesv6 = strings.Split(rawv6.String(), "\n")
		remainingv6Chains := make(map[string]bool)
		for _, line := range linesv6 {
			if strings.HasPrefix(line, ":") {
				remainingv6Chains[strings.TrimSpace(line)] = true
			}
		}
		expectv6DeletedChains := []string{"KUBE-HP-4YVONL46AKYWSKS3", "KUBE-HP-7THKRFSEH4GIIXK7", "KUBE-HP-5N7UH5JAXCVP5UJR"}
		for _, chain := range expectv6DeletedChains {
			_, ok := remainingv6Chains[chain]
			Expect(ok).To(BeFalse())
		}
	})
})
