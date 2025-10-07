// Package config contains variables which can be adjusted at build time.
package config

var (
	// RegistriesConfPath is the default path for registries.conf.
	RegistriesConfPath = "/etc/containers/registries.conf"

	// AuthDir is the default path for CRI-O's namespaced auth feature.
	AuthDir = "/etc/crio/auth"

	// KubeletAuthFilePath is the main path for the kubelet global auth file.
	KubeletAuthFilePath = "/var/lib/kubelet/config.json"
)
