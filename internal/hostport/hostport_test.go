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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	utiliptables "github.com/cri-o/cri-o/internal/iptables"
)

var _ = t.Describe("HostPort", func() {
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
})
