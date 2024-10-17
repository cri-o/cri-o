package container

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/namespace"
)

// SandboxIFace represents an interface for interacting with sandbox related information.
// This is to ensure we don't import the lib sandbox and just use its methods.
type SandboxIFace interface {
	// LogDir returns the location of the logging directory for the sandbox.
	LogDir() string

	// Annotations returns a list of annotations for the sandbox.
	Annotations() map[string]string

	// ID returns the id of the sandbox.
	ID() string

	// Name returns the name of the sandbox.
	Name() string

	// ResolvPath returns the resolv path for the sandbox.
	ResolvPath() string

	// IPs returns the ip of the sandbox.
	IPs() []string

	// NamespacePaths returns all the paths of the namespaces of the sandbox.
	NamespacePaths() []*namespace.ManagedNamespace

	// PidNsPath returns the path to the pid namespace of the sandbox.
	PidNsPath() string

	// NamespaceOptions returns the namespace options for the sandbox.
	NamespaceOptions() *types.NamespaceOption
}
