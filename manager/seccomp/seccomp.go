// +build seccomp

package seccomp

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/docker/docker/pkg/stringutils"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	libseccomp "github.com/seccomp/libseccomp-golang"
)

// LoadProfileFromStruct takes a Seccomp struct and setup seccomp in the spec.
func LoadProfileFromStruct(config Seccomp, specgen *generate.Generator) error {
	return setupSeccomp(&config, specgen)
}

// LoadProfileFromBytes takes a byte slice and decodes the seccomp profile.
func LoadProfileFromBytes(body []byte, specgen *generate.Generator) error {
	var config Seccomp
	if err := json.Unmarshal(body, &config); err != nil {
		return fmt.Errorf("decoding seccomp profile failed: %v", err)
	}
	return setupSeccomp(&config, specgen)
}

var nativeToSeccomp = map[string]Arch{
	"amd64":       ArchX86_64,
	"arm64":       ArchAARCH64,
	"mips64":      ArchMIPS64,
	"mips64n32":   ArchMIPS64N32,
	"mipsel64":    ArchMIPSEL64,
	"mipsel64n32": ArchMIPSEL64N32,
	"s390x":       ArchS390X,
}

func setupSeccomp(config *Seccomp, specgen *generate.Generator) error {
	if config == nil {
		return nil
	}

	// No default action specified, no syscalls listed, assume seccomp disabled
	if config.DefaultAction == "" && len(config.Syscalls) == 0 {
		return nil
	}

	var arch string
	var native, err = libseccomp.GetNativeArch()
	if err == nil {
		arch = native.String()
	}

	if len(config.Architectures) != 0 && len(config.ArchMap) != 0 {
		return errors.New("'architectures' and 'archMap' were specified in the seccomp profile, use either 'architectures' or 'archMap'")
	}

	customspec := specgen.Spec()
	customspec.Linux.Seccomp = &specs.Seccomp{}

	// if config.Architectures == 0 then libseccomp will figure out the architecture to use
	if len(config.Architectures) != 0 {
		for _, a := range config.Architectures {
			customspec.Linux.Seccomp.Architectures = append(customspec.Linux.Seccomp.Architectures, specs.Arch(a))
		}
	}

	if len(config.ArchMap) != 0 {
		for _, a := range config.ArchMap {
			seccompArch, ok := nativeToSeccomp[arch]
			if ok {
				if a.Arch == seccompArch {
					customspec.Linux.Seccomp.Architectures = append(customspec.Linux.Seccomp.Architectures, specs.Arch(a.Arch))
					for _, sa := range a.SubArches {
						customspec.Linux.Seccomp.Architectures = append(customspec.Linux.Seccomp.Architectures, specs.Arch(sa))
					}
					break
				}
			}
		}
	}

	customspec.Linux.Seccomp.DefaultAction = specs.Action(config.DefaultAction)

Loop:
	// Loop through all syscall blocks and convert them to libcontainer format after filtering them
	for _, call := range config.Syscalls {
		if len(call.Excludes.Arches) > 0 {
			if stringutils.InSlice(call.Excludes.Arches, arch) {
				continue Loop
			}
		}
		if len(call.Excludes.Caps) > 0 {
			for _, c := range call.Excludes.Caps {
				if stringutils.InSlice(customspec.Process.Capabilities, c) {
					continue Loop
				}
			}
		}
		if len(call.Includes.Arches) > 0 {
			if !stringutils.InSlice(call.Includes.Arches, arch) {
				continue Loop
			}
		}
		if len(call.Includes.Caps) > 0 {
			for _, c := range call.Includes.Caps {
				if !stringutils.InSlice(customspec.Process.Capabilities, c) {
					continue Loop
				}
			}
		}

		if call.Name != "" && len(call.Names) != 0 {
			return errors.New("'name' and 'names' were specified in the seccomp profile, use either 'name' or 'names'")
		}

		if call.Name != "" {
			customspec.Linux.Seccomp.Syscalls = append(customspec.Linux.Seccomp.Syscalls, createSpecsSyscall(call.Name, call.Action, call.Args))
		}

		for _, n := range call.Names {
			customspec.Linux.Seccomp.Syscalls = append(customspec.Linux.Seccomp.Syscalls, createSpecsSyscall(n, call.Action, call.Args))
		}
	}

	return nil
}

func createSpecsSyscall(name string, action Action, args []*Arg) specs.Syscall {
	newCall := specs.Syscall{
		Name:   name,
		Action: specs.Action(action),
	}

	// Loop through all the arguments of the syscall and convert them
	for _, arg := range args {
		newArg := specs.Arg{
			Index:    arg.Index,
			Value:    arg.Value,
			ValueTwo: arg.ValueTwo,
			Op:       specs.Operator(arg.Op),
		}

		newCall.Args = append(newCall.Args, newArg)
	}
	return newCall
}
