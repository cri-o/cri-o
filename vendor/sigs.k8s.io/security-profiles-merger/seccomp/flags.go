/*
Copyright The Kubernetes Authors.

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

package seccomp

import (
	"slices"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
)

// flagPolarity classifies a seccomp filter flag by what setting it does to
// the confined process, which decides how a merge combines it.
type flagPolarity int

const (
	// flagPermissive loosens confinement: SECCOMP_FILTER_FLAG_SPEC_ALLOW
	// disables the Speculative Store Bypass mitigation. Intersection keeps
	// such a flag only if every input sets it; union keeps it if any does.
	flagPermissive flagPolarity = iota
	// flagHardening tightens confinement or auditing:
	// SECCOMP_FILTER_FLAG_LOG logs every action other than allow.
	// Intersection keeps such a flag if any input sets it; union only if
	// every input does. Unknown flags are treated as hardening, which is
	// the conservative choice for intersection.
	flagHardening
	// flagListener changes how notifications are delivered:
	// SECCOMP_FILTER_FLAG_WAIT_KILLABLE_RECV only matters together with a
	// listener, which is taken from the first profile that sets one, so the
	// flag is taken from that profile too.
	flagListener
)

func polarityOf(flag specs.LinuxSeccompFlag) flagPolarity {
	switch flag {
	case specs.LinuxSeccompFlagSpecAllow:
		return flagPermissive
	case specs.LinuxSeccompFlagWaitKillableRecv:
		return flagListener
	case specs.LinuxSeccompFlagLog:
		return flagHardening
	default:
		return flagHardening
	}
}

// keepFlag decides whether a flag survives a merge given where it is set.
// A permissive flag under intersection and a hardening flag under union
// need every profile; the other two combinations need any profile. A
// listener flag belongs to the listener, so it follows the profile the
// listener comes from and is dropped when that profile does not set it.
func keepFlag(polarity flagPolarity, inLeft, inRight, intersect, listenerFromLeft bool) bool {
	if polarity == flagListener {
		if listenerFromLeft {
			return inLeft
		}

		return inRight
	}

	needsEvery := (polarity == flagPermissive) == intersect
	if needsEvery {
		return inLeft && inRight
	}

	return inLeft || inRight
}

// mergeFlags combines the flag lists of two profiles according to each
// flag's polarity. An empty list means "no flags".
func mergeFlags(
	left, right []specs.LinuxSeccompFlag, intersect, listenerFromLeft bool,
) []specs.LinuxSeccompFlag {
	var result []specs.LinuxSeccompFlag

	for _, flag := range merge.UnionSlice(left, right) {
		inLeft := slices.Contains(left, flag)
		inRight := slices.Contains(right, flag)

		if keepFlag(polarityOf(flag), inLeft, inRight, intersect, listenerFromLeft) {
			result = append(result, flag)
		}
	}

	slices.Sort(result)

	return result
}
