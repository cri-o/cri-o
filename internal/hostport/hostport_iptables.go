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
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	utilexec "k8s.io/utils/exec"

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

type hostportManagerIPTables struct {
	iptables utiliptables.Interface
	mu       sync.Mutex
}

// newHostportManagerIPTables creates a new iptables HostPortManager.
func newHostportManagerIPTables(ctx context.Context, protocol utiliptables.Protocol) (*hostportManagerIPTables, error) {
	h := &hostportManagerIPTables{
		iptables: utiliptables.New(ctx, utilexec.New(), protocol),
	}
	if !h.iptables.Present() {
		return nil, errors.New("iptables not available")
	}

	return h, nil
}

func (hm *hostportManagerIPTables) Add(id, name, podIP string, hostportMappings []*PortMapping) (err error) {
	if err := ensureKubeHostportChains(hm.iptables); err != nil {
		return err
	}

	// Ensure atomicity for iptables operations
	hm.mu.Lock()
	defer hm.mu.Unlock()

	natChains := bytes.NewBuffer(nil)
	natRules := bytes.NewBuffer(nil)

	writeLine(natChains, "*nat")

	existingChains, existingRules, err := getExistingHostportIPTablesRules(hm.iptables)
	if err != nil {
		return err
	}

	newChains := []utiliptables.Chain{}

	for _, pm := range hostportMappings {
		protocol := strings.ToLower(string(pm.Protocol))
		hpChain := getHostportChain(kubeHostportChainPrefix, id, pm)
		masqChain := getHostportChain(crioMasqueradeChainPrefix, id, pm)
		newChains = append(newChains, hpChain, masqChain)

		// Add new hostport chain
		writeLine(natChains, utiliptables.MakeChainLine(hpChain))
		writeLine(natChains, utiliptables.MakeChainLine(masqChain))

		// Prepend the new chains to KUBE-HOSTPORTS and CRIO-HOSTPORTS-MASQ
		// This avoids any leaking iptables rules that take up the same port
		writeLine(natRules, "-I", string(kubeHostportsChain),
			"-m", "comment", "--comment", fmt.Sprintf(`"%s hostport %d"`, name, pm.HostPort),
			"-m", protocol, "-p", protocol, "--dport", strconv.Itoa(int(pm.HostPort)),
			"-j", string(hpChain),
		)
		writeLine(natRules, "-I", string(crioMasqueradeChain),
			"-m", "comment", "--comment", fmt.Sprintf(`"%s hostport %d"`, name, pm.HostPort),
			"-j", string(masqChain),
		)

		// DNAT to the podIP:containerPort
		hostPortBinding := net.JoinHostPort(podIP, strconv.Itoa(int(pm.ContainerPort)))
		if pm.HostIP == "" || pm.HostIP == "0.0.0.0" || pm.HostIP == "::" {
			writeLine(natRules, "-A", string(hpChain),
				"-m", "comment", "--comment", fmt.Sprintf(`"%s hostport %d"`, name, pm.HostPort),
				"-m", protocol, "-p", protocol,
				"-j", "DNAT", "--to-destination="+hostPortBinding)
		} else {
			writeLine(natRules, "-A", string(hpChain),
				"-m", "comment", "--comment", fmt.Sprintf(`"%s hostport %d"`, name, pm.HostPort),
				"-m", protocol, "-p", protocol, "-d", pm.HostIP,
				"-j", "DNAT", "--to-destination="+hostPortBinding)
		}

		// SNAT hairpin traffic. There is no "ctorigaddrtype" so we can't
		// _exactly_ match only the traffic that was definitely DNATted by our
		// rule as opposed to someone else's. But if the traffic has been DNATted
		// and has src=dst=podIP then _someone_ needs to masquerade it, and the
		// worst case here is just that "-j MASQUERADE" gets called twice.
		writeLine(natRules, "-A", string(masqChain),
			"-m", "comment", "--comment", fmt.Sprintf(`"%s hostport %d"`, name, pm.HostPort),
			"-m", "conntrack", "--ctorigdstport", strconv.Itoa(int(pm.HostPort)),
			"-m", protocol, "-p", protocol, "--dport", strconv.Itoa(int(pm.ContainerPort)),
			"-s", podIP, "-d", podIP,
			"-j", "MASQUERADE")
	}

	// getHostportChain should be able to provide unique hostport chain name using hash
	// if there is a chain conflict or multiple Adds have been triggered for a single pod,
	// filtering should be able to avoid further problem
	filterChains(existingChains, newChains)
	existingRules = filterRules(existingRules, newChains)

	for _, chain := range existingChains {
		writeLine(natChains, chain)
	}

	for _, rule := range existingRules {
		writeLine(natRules, rule)
	}

	writeLine(natRules, "COMMIT")

	return hm.syncIPTables(append(natChains.Bytes(), natRules.Bytes()...))
}

func (hm *hostportManagerIPTables) Remove(id string, hostportMappings []*PortMapping) (err error) {
	// Ensure atomicity for iptables operations
	hm.mu.Lock()
	defer hm.mu.Unlock()

	var existingChains map[utiliptables.Chain]string

	var existingRules []string

	existingChains, existingRules, err = getExistingHostportIPTablesRules(hm.iptables)
	if err != nil {
		return err
	}

	// Gather target hostport chains for removal
	chainsToRemove := []utiliptables.Chain{}
	for _, pm := range hostportMappings {
		chainsToRemove = append(chainsToRemove,
			getHostportChain(kubeHostportChainPrefix, id, pm),
			getHostportChain(crioMasqueradeChainPrefix, id, pm),
		)
	}

	// remove rules that consists of target chains
	remainingRules := filterRules(existingRules, chainsToRemove)

	// gather target hostport chains that exists in iptables-save result
	existingChainsToRemove := []utiliptables.Chain{}

	for _, chain := range chainsToRemove {
		if _, ok := existingChains[chain]; ok {
			existingChainsToRemove = append(existingChainsToRemove, chain)
		}
	}

	// exit if there is nothing to remove
	if len(existingChainsToRemove) == 0 {
		return nil
	}

	natChains := bytes.NewBuffer(nil)
	natRules := bytes.NewBuffer(nil)

	writeLine(natChains, "*nat")

	for _, chain := range existingChains {
		writeLine(natChains, chain)
	}

	for _, rule := range remainingRules {
		writeLine(natRules, rule)
	}

	for _, chain := range existingChainsToRemove {
		writeLine(natRules, "-X", string(chain))
	}

	writeLine(natRules, "COMMIT")

	return hm.syncIPTables(append(natChains.Bytes(), natRules.Bytes()...))
}

// syncIPTables executes iptables-restore with given lines.
func (hm *hostportManagerIPTables) syncIPTables(lines []byte) error {
	logrus.Infof("Restoring iptables rules: %s", lines)

	err := hm.iptables.RestoreAll(lines, utiliptables.NoFlushTables, utiliptables.RestoreCounters)
	if err != nil {
		return fmt.Errorf("failed to execute iptables-restore: %w", err)
	}

	return nil
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

// getHostportChain takes id, hostport and protocol for a pod and returns associated iptables chain.
// This is computed by hashing (sha256) then encoding to base32 and truncating, and prepending
// the prefix. We do this because IPTables Chain Names must be <= 28 chars long, and the longer
// they are the harder they are to read.
// WARNING: Please do not change this function. Otherwise, HostportManager may not be able to
// identify existing iptables chains.
func getHostportChain(prefix, id string, pm *PortMapping) utiliptables.Chain {
	hash := sha256.Sum256([]byte(id + strconv.Itoa(int(pm.HostPort)) + string(pm.Protocol) + pm.HostIP))
	encoded := base32.StdEncoding.EncodeToString(hash[:])

	return utiliptables.Chain(prefix + encoded[:16])
}

// getExistingHostportIPTablesRules retrieves raw data from iptables-save, parse it,
// return all the hostport related chains and rules
//
//nolint:gocritic // unnamedResult: consider giving a name to these results
func getExistingHostportIPTablesRules(iptables utiliptables.Interface) (map[utiliptables.Chain]string, []string, error) {
	iptablesData := bytes.NewBuffer(nil)

	err := iptables.SaveInto(utiliptables.TableNAT, iptablesData)
	if err != nil { // if we failed to get any rules
		return nil, nil, fmt.Errorf("failed to execute iptables-save: %w", err)
	}

	existingNATChains := getChainLines(utiliptables.TableNAT, iptablesData.Bytes())

	existingHostportChains := make(map[utiliptables.Chain]string)
	existingHostportRules := []string{}

	for chain := range existingNATChains {
		if chain == kubeHostportsChain || chain == crioMasqueradeChain ||
			strings.HasPrefix(string(chain), kubeHostportChainPrefix) ||
			strings.HasPrefix(string(chain), crioMasqueradeChainPrefix) {
			existingHostportChains[chain] = string(existingNATChains[chain])
		}
	}

	for _, line := range strings.Split(iptablesData.String(), "\n") {
		if strings.HasPrefix(line, "-A "+kubeHostportChainPrefix) ||
			strings.HasPrefix(line, "-A "+crioMasqueradeChainPrefix) ||
			strings.HasPrefix(line, fmt.Sprintf("-A %s ", string(kubeHostportsChain))) ||
			strings.HasPrefix(line, fmt.Sprintf("-A %s ", string(crioMasqueradeChain))) {
			existingHostportRules = append(existingHostportRules, line)
		}
	}

	return existingHostportChains, existingHostportRules, nil
}

// getChainLines parses a table's iptables-save data to find chains in the table.
// It returns a map of iptables.Chain to []byte where the []byte is the chain line
// from save (with counters etc.).
// Note that to avoid allocations memory is SHARED with save.
func getChainLines(table utiliptables.Table, save []byte) map[utiliptables.Chain][]byte {
	chainsMap := make(map[utiliptables.Chain][]byte)
	tablePrefix := []byte("*" + string(table))
	readIndex := 0
	// find beginning of table
	for readIndex < len(save) {
		line, n := readLine(readIndex, save)
		readIndex = n

		if bytes.HasPrefix(line, tablePrefix) {
			break
		}
	}

	var (
		commitBytes = []byte("COMMIT")
		spaceBytes  = []byte(" ")
	)
	// parse table lines
	for readIndex < len(save) {
		line, n := readLine(readIndex, save)
		readIndex = n

		if len(line) == 0 {
			continue
		}

		if bytes.HasPrefix(line, commitBytes) || line[0] == '*' { //nolint:gocritic
			break
		} else if line[0] == '#' {
			continue
		} else if line[0] == ':' && len(line) > 1 {
			// We assume that the <line> contains space - chain lines have 3 fields,
			// space delimited. If there is no space, this line will panic.
			spaceIndex := bytes.Index(line, spaceBytes)
			if spaceIndex == -1 {
				panic(fmt.Sprintf("Unexpected chain line in iptables-save output: %v", string(line)))
			}

			chain := utiliptables.Chain(line[1:spaceIndex])
			chainsMap[chain] = line
		}
	}

	return chainsMap
}

func readLine(readIndex int, byteArray []byte) (line []byte, n int) {
	currentReadIndex := readIndex

	// consume left spaces
	for currentReadIndex < len(byteArray) {
		if byteArray[currentReadIndex] == ' ' {
			currentReadIndex++
		} else {
			break
		}
	}

	// leftTrimIndex stores the left index of the line after the line is left-trimmed
	leftTrimIndex := currentReadIndex

	// rightTrimIndex stores the right index of the line after the line is right-trimmed
	// it is set to -1 since the correct value has not yet been determined.
	rightTrimIndex := -1

	for ; currentReadIndex < len(byteArray); currentReadIndex++ {
		if byteArray[currentReadIndex] == ' ' { //nolint:gocritic
			// set rightTrimIndex
			if rightTrimIndex == -1 {
				rightTrimIndex = currentReadIndex
			}
		} else if (byteArray[currentReadIndex] == '\n') || (currentReadIndex == (len(byteArray) - 1)) {
			// end of line or byte buffer is reached
			if currentReadIndex <= leftTrimIndex {
				return nil, currentReadIndex + 1
			}
			// set the rightTrimIndex
			if rightTrimIndex == -1 {
				rightTrimIndex = currentReadIndex
				if currentReadIndex == (len(byteArray)-1) && (byteArray[currentReadIndex] != '\n') {
					// ensure that the last character is part of the returned string,
					// unless the last character is '\n'
					rightTrimIndex = currentReadIndex + 1
				}
			}
			// Avoid unnecessary allocation.
			return byteArray[leftTrimIndex:rightTrimIndex], currentReadIndex + 1
		} else {
			// unset rightTrimIndex
			rightTrimIndex = -1
		}
	}

	return nil, currentReadIndex
}

// filterRules filters input rules with input chains. Rules that did not involve any filter chain will be returned.
// The order of the input rules is important and is preserved.
func filterRules(rules []string, filters []utiliptables.Chain) []string {
	filtered := []string{}

	for _, rule := range rules {
		skip := false

		for _, filter := range filters {
			if strings.Contains(rule, string(filter)) {
				skip = true

				break
			}
		}

		if !skip {
			filtered = append(filtered, rule)
		}
	}

	return filtered
}

// filterChains deletes all entries of filter chains from chain map.
func filterChains(chains map[utiliptables.Chain]string, filterChains []utiliptables.Chain) {
	for _, chain := range filterChains {
		delete(chains, chain)
	}
}

// Join all words with spaces, terminate with newline and write to buf.
func writeLine(buf *bytes.Buffer, words ...string) {
	buf.WriteString(strings.Join(words, " ") + "\n")
}
