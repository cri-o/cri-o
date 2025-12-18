//go:build linux

package oci

import (
	"context"
	"errors"
	"time"

	cgroupsV1 "github.com/containerd/cgroups/stats/v1"
	cgroupsV2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/containerd/api/runtime/task/v2"
	"github.com/containerd/typeurl"
	"github.com/opencontainers/cgroups"

	"github.com/cri-o/cri-o/internal/lib/stats"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/utils/errdefs"
)

// CgroupStats provides cgroup statistics of a container.
func (r *runtimeVM) CgroupStats(ctx context.Context, c *Container, _ string) (*stats.CgroupStats, error) {
	log.Debugf(ctx, "RuntimeVM.CgroupStats() start")
	defer log.Debugf(ctx, "RuntimeVM.CgroupStats() end")

	// Lock the container with a shared lock
	c.opLock.RLock()
	defer c.opLock.RUnlock()

	resp, err := r.task.Stats(r.ctx, &task.StatsRequest{
		ID: c.ID(),
	})
	if err != nil {
		return nil, errdefs.FromGRPC(err)
	}

	if resp == nil {
		return nil, errors.New("could not retrieve container stats")
	}

	statsData, err := typeurl.UnmarshalAny(resp.GetStats())
	if err != nil {
		return nil, err
	}

	// We can't assume the version of metrics we will get based on the host system,
	// because the guest VM may be using a different version.
	// Trying to retrieve the V1 metrics first, and if it fails, try the v2

	m, ok := statsData.(*cgroupsV1.Metrics)
	if ok {
		return metricsV1ToCgroupStats(ctx, m), nil
	} else {
		m, ok := statsData.(*cgroupsV2.Metrics)
		if ok {
			return metricsV2ToCgroupStats(ctx, m), nil
		} else {
			return nil, errors.New("unknown stats type")
		}
	}
}

func (r *runtimeVM) DiskStats(ctx context.Context, c *Container, _ string) (*stats.DiskStats, error) {
	return &stats.DiskStats{}, nil
}

func metricsV1ToCgroupStats(ctx context.Context, m *cgroupsV1.Metrics) *stats.CgroupStats {
	_ = ctx // unused but kept for consistency with V2 function

	hugetlbStats := map[string]cgroups.HugetlbStats{}
	for _, hugetlb := range m.Hugetlb {
		hugetlbStats[hugetlb.Pagesize] = cgroups.HugetlbStats{
			Usage:    hugetlb.Usage,
			MaxUsage: hugetlb.Max,
		}
	}

	memStats := map[string]uint64{
		"total_inactive_file": m.Memory.TotalInactiveFile,
		"total_rss":           m.Memory.RSS,
		"mapped_file":         m.Memory.MappedFile,
		"total_mapped_file":   m.Memory.TotalMappedFile,
	}

	return &stats.CgroupStats{
		Stats: cgroups.Stats{
			CpuStats: cgroups.CpuStats{
				CpuUsage: cgroups.CpuUsage{
					TotalUsage:        m.CPU.Usage.Total,
					PercpuUsage:       m.CPU.Usage.PerCPU,
					UsageInKernelmode: m.CPU.Usage.Kernel,
					UsageInUsermode:   m.CPU.Usage.User,
				},
				ThrottlingData: cgroups.ThrottlingData{
					Periods:          m.CPU.Throttling.Periods,
					ThrottledPeriods: m.CPU.Throttling.ThrottledPeriods,
					ThrottledTime:    m.CPU.Throttling.ThrottledTime,
				},
			},
			MemoryStats: cgroups.MemoryStats{
				Cache: m.Memory.Cache,
				Usage: cgroups.MemoryData{
					Usage:    m.Memory.Usage.Usage,
					MaxUsage: m.Memory.Usage.Max,
					Failcnt:  m.Memory.Usage.Failcnt,
					Limit:    m.Memory.Usage.Limit,
				},
				SwapUsage: cgroups.MemoryData{
					Usage:    m.Memory.Swap.Usage,
					MaxUsage: m.Memory.Swap.Max,
					Failcnt:  m.Memory.Swap.Failcnt,
					Limit:    m.Memory.Swap.Limit,
				},
				KernelUsage: cgroups.MemoryData{
					Usage: m.Memory.Kernel.Usage,
				},
				KernelTCPUsage: cgroups.MemoryData{
					Usage: m.Memory.KernelTCP.Usage,
				},
				Stats: memStats,
			},
			PidsStats: cgroups.PidsStats{
				Current: m.Pids.Current,
				Limit:   m.Pids.Limit,
			},
			HugetlbStats: hugetlbStats,
		},
		SystemNano: time.Now().UnixNano(),
	}
}

func metricsV2ToCgroupStats(ctx context.Context, m *cgroupsV2.Metrics) *stats.CgroupStats {
	_ = ctx // unused but kept for interface consistency

	hugetlbStats := map[string]cgroups.HugetlbStats{}
	for _, hugetlb := range m.Hugetlb {
		hugetlbStats[hugetlb.Pagesize] = cgroups.HugetlbStats{
			Usage:    hugetlb.Current,
			MaxUsage: hugetlb.Max,
		}
	}

	// For cgroup v2, create the Stats map with the appropriate keys
	memStats := map[string]uint64{
		"inactive_file": m.Memory.InactiveFile,
		"anon":          m.Memory.Anon,
		"file_mapped":   m.Memory.FileMapped,
		"pgfault":       m.Memory.Pgfault,
		"pgmajfault":    m.Memory.Pgmajfault,
	}

	return &stats.CgroupStats{
		Stats: cgroups.Stats{
			CpuStats: cgroups.CpuStats{
				CpuUsage: cgroups.CpuUsage{
					TotalUsage:        m.CPU.UsageUsec * 1000,
					UsageInKernelmode: m.CPU.SystemUsec * 1000,
					UsageInUsermode:   m.CPU.UserUsec * 1000,
				},
				ThrottlingData: cgroups.ThrottlingData{
					Periods:          m.CPU.NrPeriods,
					ThrottledPeriods: m.CPU.NrThrottled,
					ThrottledTime:    m.CPU.ThrottledUsec * 1000,
				},
			},
			MemoryStats: cgroups.MemoryStats{
				Cache: m.Memory.File,
				Usage: cgroups.MemoryData{
					Usage: m.Memory.Usage,
					Limit: m.Memory.UsageLimit,
				},
				SwapUsage: cgroups.MemoryData{
					Usage: m.Memory.SwapUsage,
					Limit: m.Memory.SwapLimit,
				},
				KernelUsage: cgroups.MemoryData{
					Usage: m.Memory.KernelStack,
				},
				Stats: memStats,
			},
			PidsStats: cgroups.PidsStats{
				Current: m.Pids.Current,
				Limit:   m.Pids.Limit,
			},
			HugetlbStats: hugetlbStats,
		},
		SystemNano: time.Now().UnixNano(),
	}
}
