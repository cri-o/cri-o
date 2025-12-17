//go:build !linux

package statsserver

import (
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	"github.com/cri-o/cri-o/internal/oci"
)

func generateContainerPressureMetrics(ctr *oci.Container, cpu *cgmgr.CPUStats, memory *cgmgr.MemoryStats, blkio *cgmgr.DiskIOStats) []*types.Metric {
	// pressure metrics are not supported on non-linux platforms
	return nil
}
