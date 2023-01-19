//go:build !freebsd
// +build !freebsd

package system

type platformStatT struct {
}

// Flags return file flags if supported or zero otherwise
func (s StatT) Flags() uint32 {
	return 0
}
