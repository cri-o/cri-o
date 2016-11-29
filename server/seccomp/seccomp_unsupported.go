// +build !seccomp

package seccomp

import "github.com/opencontainers/runtime-tools/generate"

// LoadProfileFromStruct takes a Seccomp struct and setup seccomp in the spec.
func LoadProfileFromStruct(config Seccomp, specgen *generate.Generator) error {
	return nil
}

// LoadProfileFromBytes takes a byte slice and decodes the seccomp profile.
func LoadProfileFromBytes(body []byte, specgen *generate.Generator) error {
	return nil
}
