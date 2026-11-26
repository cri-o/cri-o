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
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"sigs.k8s.io/security-profiles-merger/internal/merge"
)

// profileJSON is specs.LinuxSeccomp as Marshal writes it: field for field,
// except that Flags is a pointer, so that a list that is set but empty is
// written rather than left out.
type profileJSON struct {
	DefaultAction    specs.LinuxSeccompAction  `json:"defaultAction"`
	DefaultErrnoRet  *uint                     `json:"defaultErrnoRet,omitempty"`
	Architectures    []specs.Arch              `json:"architectures,omitempty"`
	Flags            *[]specs.LinuxSeccompFlag `json:"flags,omitempty"`
	ListenerPath     string                    `json:"listenerPath,omitempty"`
	ListenerMetadata string                    `json:"listenerMetadata,omitempty"`
	Syscalls         []specs.LinuxSyscall      `json:"syscalls,omitempty"`
}

// Marshal encodes a profile as json.Marshal does, except that a Flags list
// that is set but empty is written as "flags": [], and '<', '>' and '&' are
// left unescaped. Runtimes set SECCOMP_FILTER_FLAG_SPEC_ALLOW for a profile
// without the member and no flag for an empty list, so write a merge result
// with Marshal (see the Flags section). It returns ErrNilProfile for a nil
// profile and [ErrUnsupportedSpecField] for one setting a field this package
// does not read, which it would not write.
func Marshal(profile *specs.LinuxSeccomp) ([]byte, error) {
	if profile == nil {
		return nil, ErrNilProfile
	}

	err := validateSpecFields(profile)
	if err != nil {
		return nil, err
	}

	written := profileJSON{
		DefaultAction:    profile.DefaultAction,
		DefaultErrnoRet:  profile.DefaultErrnoRet,
		Architectures:    profile.Architectures,
		Flags:            nil,
		ListenerPath:     profile.ListenerPath,
		ListenerMetadata: profile.ListenerMetadata,
		Syscalls:         profile.Syscalls,
	}

	if profile.Flags != nil {
		written.Flags = &profile.Flags
	}

	var out bytes.Buffer

	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)

	err = enc.Encode(written)
	if err != nil {
		return nil, fmt.Errorf("encoding profile: %w", err)
	}

	return bytes.TrimSuffix(out.Bytes(), []byte("\n")), nil
}

// Clone returns a deep copy of the profile, or nil for a nil profile. The
// copy shares no slice or pointer of the fields this package reads and keeps
// nil and empty slices apart, so it encodes to the same JSON. A field this
// package does not read ([ErrUnsupportedSpecField]) is copied as Go assigns
// it, sharing what it points to.
func Clone(profile *specs.LinuxSeccomp) *specs.LinuxSeccomp {
	if profile == nil {
		return nil
	}

	clone := *profile
	clone.DefaultErrnoRet = merge.ClonePtr(profile.DefaultErrnoRet)
	clone.Architectures = slices.Clone(profile.Architectures)
	clone.Flags = slices.Clone(profile.Flags)
	clone.Syscalls = slices.Clone(profile.Syscalls)

	for idx := range clone.Syscalls {
		entry := &clone.Syscalls[idx]
		entry.Names = slices.Clone(entry.Names)
		entry.ErrnoRet = merge.ClonePtr(entry.ErrnoRet)
		entry.Args = slices.Clone(entry.Args)
	}

	return &clone
}
