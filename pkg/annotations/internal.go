package annotations

import "strings"

// internalAnnotations is the set of annotation keys that CRI-O sets at
// runtime to persist container/sandbox state. These must never be
// settable by user input (pod spec annotations or labels).
var internalAnnotations = map[string]struct{}{
	"io.kubernetes.cri-o.Annotations":        {},
	"io.kubernetes.cri-o.ContainerID":         {},
	"io.kubernetes.cri-o.ContainerName":       {},
	"io.kubernetes.cri-o.ContainerType":       {},
	"io.kubernetes.cri-o.Created":             {},
	"io.kubernetes.cri-o.HostName":            {},
	"io.kubernetes.cri-o.CgroupParent":        {},
	"io.kubernetes.cri-o.IP":                  {},
	"io.kubernetes.cri-o.NamespaceOptions":    {},
	"io.kubernetes.cri-o.SeccompProfilePath":  {},
	"io.kubernetes.cri-o.Image":               {},
	"io.kubernetes.cri-o.ImageName":           {},
	"io.kubernetes.cri-o.ImageRef":            {},
	"io.kubernetes.cri-o.KubeName":            {},
	"io.kubernetes.cri-o.PortMappings":        {},
	"io.kubernetes.cri-o.Labels":              {},
	"io.kubernetes.cri-o.LogPath":             {},
	"io.kubernetes.cri-o.Metadata":            {},
	"io.kubernetes.cri-o.Name":                {},
	"io.kubernetes.cri-o.Namespace":           {},
	"io.kubernetes.cri-o.PrivilegedRuntime":   {},
	"io.kubernetes.cri-o.ResolvPath":          {},
	"io.kubernetes.cri-o.HostnamePath":        {},
	"io.kubernetes.cri-o.SandboxID":           {},
	"io.kubernetes.cri-o.SandboxName":         {},
	"io.kubernetes.cri-o.ShmPath":             {},
	"io.kubernetes.cri-o.MountPoint":          {},
	"io.kubernetes.cri-o.RuntimeHandler":      {},
	"io.kubernetes.cri-o.TTY":                 {},
	"io.kubernetes.cri-o.Stdin":               {},
	"io.kubernetes.cri-o.StdinOnce":           {},
	"io.kubernetes.cri-o.Volumes":             {},
	"io.kubernetes.cri-o.HostNetwork":         {},
	"io.kubernetes.cri-o.CNIResult":           {},
	"io.container.manager":                    {},
}

// IsInternal returns true if the annotation key is set by CRI-O at runtime and
// must not be overwritten by user input.
func IsInternal(key string) bool {
	_, ok := internalAnnotations[key]
	// Also match indexed variants like io.kubernetes.cri-o.IP.0, IP.1, etc.
	return ok || strings.HasPrefix(key, "io.kubernetes.cri-o.IP.")
}
