//go:build !linux

package statsserver

import (
	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/oci"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func generateContainerDiskMetrics(ctr *oci.Container, diskStats *oci.FilesystemMetrics) []*types.Metric {
	return []*types.Metric{}
}

func generateContainerDiskIOMetrics(ctr *oci.Container, ioStats *cgmgr.DiskIOStats) []*types.Metric {
	return []*types.Metric{}
}
