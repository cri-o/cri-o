package config

import (
	"strings"

	"github.com/pkg/errors"
)

// Sysctl is a generic abstraction over key value based sysctls
type Sysctl struct {
	key, value string
}

// Key returns the key of the sysctl (key=value format)
func (s *Sysctl) Key() string {
	return s.key
}

// Value returns the value of the sysctl (key=value format)
func (s *Sysctl) Value() string {
	return s.value
}

// Sysctls returns the parsed sysctl slice and an error if not parsable
func (c *RuntimeConfig) Sysctls() (sysctls []Sysctl, err error) {
	for _, sysctl := range c.DefaultSysctls {
		// skip empty values for sake of backwards compatibility
		if sysctl == "" {
			continue
		}
		split := strings.SplitN(sysctl, "=", 2)
		if len(split) == 2 {
			sysctls = append(sysctls, Sysctl{key: split[0], value: split[1]})
		} else {
			return nil, errors.Errorf("%q is not in key=value format", sysctl)
		}
	}
	return sysctls, nil
}

// Namespace represents a kernel namespace name.
type Namespace string

const (
	// IpcNamespace is the Linux IPC namespace
	IpcNamespace = Namespace("ipc")

	// NetNamespace is the network namespace
	NetNamespace = Namespace("net")

	// UnknownNamespace is the zero value if no namespace is known
	UnknownNamespace = Namespace("")
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
			return errors.Errorf(nsErrorFmt, s.Key(), ns)
		}
		return nil
	}
	for p, ns := range prefixNamespaces {
		if strings.HasPrefix(s.Key(), p) {
			if ns == IpcNamespace && hostIPC {
				return errors.Errorf(nsErrorFmt, s.Key(), ns)
			}
			if ns == NetNamespace && hostNet {
				return errors.Errorf(nsErrorFmt, s.Key(), ns)
			}
			return nil
		}
	}
	return errors.Errorf("%s not whitelisted", s.Key())
}
