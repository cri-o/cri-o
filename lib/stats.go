package lib

import (
	"github.com/kubernetes-incubator/cri-o/oci"
)

// ContainerStats contains the statistics information for a running container
type ContainerStats struct {
	Container   string
	CPU         float64
	CPUNano     uint64
	SystemNano  int64
	MemUsage    uint64
	MemLimit    uint64
	MemPerc     float64
	NetInput    uint64
	NetOutput   uint64
	BlockInput  uint64
	BlockOutput uint64
	PIDs        uint64
}

// GetContainerStats gets the running stats for a given container
func (c *ContainerServer) GetContainerStats(ctr *oci.Container, previousStats *ContainerStats) (*ContainerStats, error) {
	return c.getContainerStats(ctr, previousStats)
}
