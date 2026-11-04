package seccomp

import (
	"encoding/json"
	"os"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
)

// SaveSpecToFile writes the runtime spec of the generator to path, as its
// SaveToFile method does, but keeps a seccomp flags list that is set and
// empty: the runtime spec leaves such a list out when marshaled, and runc and
// crun set SECCOMP_FILTER_FLAG_SPEC_ALLOW for a profile without the member,
// which the empty list turns off. An intersection with a profile that turns
// the flag off results in that list (see the flags of security-profiles-merger).
func SaveSpecToFile(g *generate.Generator, path string) error {
	if !hasEmptyFlags(g.Config) {
		return g.SaveToFile(path, generate.ExportOptions{})
	}

	data, err := MarshalSpec(g.Config)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}

// MarshalSpec marshals a runtime spec, with a seccomp flags list that is set
// and empty as "flags": [] (see SaveSpecToFile).
func MarshalSpec(spec *rspec.Spec) ([]byte, error) {
	if !hasEmptyFlags(spec) {
		return json.MarshalIndent(spec, "", "\t")
	}

	// A field of the outer struct takes precedence over the field of the
	// same name of an embedded one, so each level replaces one member and
	// leaves the others to the type of the runtime spec.
	type seccompWithFlags struct {
		*rspec.LinuxSeccomp

		Flags []rspec.LinuxSeccompFlag `json:"flags"`
	}

	type linuxWithSeccomp struct {
		*rspec.Linux

		Seccomp seccompWithFlags `json:"seccomp"`
	}

	type specWithLinux struct {
		*rspec.Spec

		Linux linuxWithSeccomp `json:"linux"`
	}

	return json.MarshalIndent(specWithLinux{
		Spec: spec,
		Linux: linuxWithSeccomp{
			Linux: spec.Linux,
			Seccomp: seccompWithFlags{
				LinuxSeccomp: spec.Linux.Seccomp,
				Flags:        spec.Linux.Seccomp.Flags,
			},
		},
	}, "", "\t")
}

// hasEmptyFlags returns true if the seccomp profile of the spec has a flags
// list that is set and empty.
func hasEmptyFlags(spec *rspec.Spec) bool {
	if spec == nil || spec.Linux == nil || spec.Linux.Seccomp == nil {
		return false
	}

	flags := spec.Linux.Seccomp.Flags

	return flags != nil && len(flags) == 0
}
