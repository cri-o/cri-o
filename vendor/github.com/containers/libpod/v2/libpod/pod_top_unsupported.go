// +build !linux

package libpod

import "github.com/containers/libpod/v2/libpod/define"

// GetPodPidInformation is exclusive to linux
func (p *Pod) GetPodPidInformation(descriptors []string) ([]string, error) {
	return nil, define.ErrNotImplemented
}
