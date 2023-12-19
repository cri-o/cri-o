//go:build freebsd
// +build freebsd

package ocicni

const (
	// DefaultConfDir is the default place to look for CNI Network
	DefaultConfDir = "/usr/local/etc/cni/net.d"
	// DefaultBinDir is the default place to look for CNI config files
	DefaultBinDir = "/usr/local/libexec/cni"
)
