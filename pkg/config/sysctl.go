package config

import (
	"fmt"
	"strings"
)

func NewSysctl(key, value string) *Sysctl {
	return &Sysctl{key, value}
}

// Sysctl is a generic abstraction over key value based sysctls.
type Sysctl struct {
	key, value string
}

// Key returns the key of the sysctl (key=value format).
func (s *Sysctl) Key() string {
	return s.key
}

// Value returns the value of the sysctl (key=value format).
func (s *Sysctl) Value() string {
	return s.value
}

// Sysctls returns the parsed sysctl slice and an error if not parsable
// Some validation based on https://go.podman.io/common/blob/main/pkg/sysctl/sysctl.go
func (c *RuntimeConfig) Sysctls() ([]Sysctl, error) {
	sysctls := make([]Sysctl, 0, len(c.DefaultSysctls))

	for _, sysctl := range c.DefaultSysctls {
		// skip empty values for sake of backwards compatibility
		if sysctl == "" {
			continue
		}

		split := strings.SplitN(sysctl, "=", 2)
		if len(split) != 2 {
			return nil, fmt.Errorf("%q is not in key=value format", sysctl)
		}

		// pinns nor runc expect sysctls of the form 'key = value', but rather
		// 'key=value'
		trimmed := strings.TrimSpace(split[0]) + "=" + strings.TrimSpace(split[1])
		if trimmed != sysctl {
			return nil, fmt.Errorf("'%s' is invalid, extra spaces found: format should be key=value", sysctl)
		}

		sysctls = append(sysctls, Sysctl{key: split[0], value: split[1]})
	}

	return sysctls, nil
}

// Namespace represents a kernel namespace name.
type Namespace string

const (
	// IpcNamespace is the Linux IPC namespace.
	IpcNamespace = Namespace("ipc")

	// NetNamespace is the network namespace.
	NetNamespace = Namespace("net")
)

var namespaces = map[string]Namespace{
	"kernel.sem": IpcNamespace,
}

var prefixNamespaces = map[string]Namespace{
	"kernel.shm": IpcNamespace,
	"kernel.msg": IpcNamespace,
	"fs.mqueue.": IpcNamespace,
	"net.":       NetNamespace,
}

// Validate checks that a sysctl is whitelisted because it is known to be
// namespaced by the Linux kernel. The parameters hostNet and hostIPC are used
// to forbid sysctls for pod sharing the respective namespaces with the host.
// This check is only used on sysctls defined by the user in the crio.conf
// file.
func (s *Sysctl) Validate(hostNet, hostIPC bool) error {
	nsErrorFmt := "%q not allowed with host %s enabled"

	if ns, found := namespaces[s.Key()]; found {
		if ns == IpcNamespace && hostIPC {
			return fmt.Errorf(nsErrorFmt, s.Key(), ns)
		}

		return nil
	}

	for p, ns := range prefixNamespaces {
		if strings.HasPrefix(s.Key(), p) {
			if ns == IpcNamespace && hostIPC {
				return fmt.Errorf(nsErrorFmt, s.Key(), ns)
			}

			if ns == NetNamespace && hostNet {
				return fmt.Errorf(nsErrorFmt, s.Key(), ns)
			}

			return nil
		}
	}

	return fmt.Errorf("%s not whitelisted", s.Key())
}
