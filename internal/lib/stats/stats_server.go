package statsserver

import (
	"context"
	"fmt"
	"sync"
	"time"

	cstorage "github.com/containers/storage"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/config"
)

// StatsServer is responsible for maintaining a list of container and sandbox stats.
// If collectionPeriod is > 0, it maintains this list by updating the stats on collectionPeriod frequency.
// Otherwise, it only updates the stats as they're requested.
type StatsServer struct {
	shutdown         chan struct{}
	alreadyShutdown  bool
	collectionPeriod time.Duration
	sboxStats        map[string]*types.PodSandboxStats
	ctrStats         map[string]*types.ContainerStats
	sboxMetrics      map[string]*SandboxMetrics
	ctx              context.Context
	parentServerIface
	mutex sync.Mutex
}

// parentServerIface is an interface for requesting information from the parent ContainerServer.
// This is to be able to decouple stats server from the ContainerServer package, while also preventing
// data duplication (mainly in the active list of sandboxes), and avoid circular dependencies to boot.
type parentServerIface interface {
	Runtime() *oci.Runtime
	Store() cstorage.Store
	ListSandboxes() []*sandbox.Sandbox
	GetSandbox(string) *sandbox.Sandbox
	Config() *config.Config
}

// New returns a new StatsServer, deriving the needed information from the provided parentServerIface.
func New(ctx context.Context, cs parentServerIface) *StatsServer {
	ss := &StatsServer{
		shutdown:          make(chan struct{}, 1),
		alreadyShutdown:   false,
		collectionPeriod:  time.Duration(cs.Config().CollectionPeriod) * time.Second,
		sboxStats:         make(map[string]*types.PodSandboxStats),
		ctrStats:          make(map[string]*types.ContainerStats),
		sboxMetrics:       make(map[string]*SandboxMetrics),
		parentServerIface: cs,
		ctx:               ctx,
	}
	go ss.updateLoop()
	return ss
}

// updateLoop updates the current list of stats every collectionPeriod seconds.
// If collectionPeriod is 0, it does nothing.
func (ss *StatsServer) updateLoop() {
	if ss.collectionPeriod == 0 {
		// fetch stats on-demand
		return
	}
	for {
		select {
		case <-ss.shutdown:
			return
		case <-time.After(ss.collectionPeriod):
		}
		ss.update()
	}
}

// update updates the list of container and sandbox stats.
// It does so by updating the stats of every sandbox, which in turn
// updates the stats for each container it has.
func (ss *StatsServer) update() {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	for _, sb := range ss.ListSandboxes() {
		ss.updateSandbox(sb)
	}
}

// updateUsageNanoCores calculates the usage nano cores by averaging the CPU usage between the timestamps
// of the old usage and the recently gathered usage.
func updateUsageNanoCores(old, current *types.CpuUsage) {
	if old == nil || current == nil || old.UsageCoreNanoSeconds == nil || current.UsageCoreNanoSeconds == nil {
		return
	}

	nanoSeconds := current.Timestamp - old.Timestamp

	usageNanoCores := uint64(float64(current.UsageCoreNanoSeconds.Value-old.UsageCoreNanoSeconds.Value) /
		float64(nanoSeconds) * float64(time.Second/time.Nanosecond))

	current.UsageNanoCores = &types.UInt64Value{
		Value: usageNanoCores,
	}
}

// populateWritableLayer attempts to populate the container stats writable layer.
func (ss *StatsServer) populateWritableLayer(stats *types.ContainerStats, container *oci.Container) {
	writableLayer, err := ss.writableLayerForContainer(container)
	if err != nil {
		logrus.Error(err)
	}
	stats.WritableLayer = writableLayer
}

// writableLayerForContainer gathers information about the container's writable layer.
// It does so by calling into the GraphDriver's endpoint to get the UsedBytes and InodesUsed.
func (ss *StatsServer) writableLayerForContainer(container *oci.Container) (*types.FilesystemUsage, error) {
	writableLayer := &types.FilesystemUsage{
		Timestamp: time.Now().UnixNano(),
		FsId:      &types.FilesystemIdentifier{Mountpoint: container.MountPoint()},
	}
	driver, err := ss.Store().GraphDriver()
	if err != nil {
		return writableLayer, fmt.Errorf("unable to get graph driver for disk usage for container %s: %w", container.ID(), err)
	}
	storageContainer, err := ss.Store().Container(container.ID())
	if err != nil {
		return writableLayer, fmt.Errorf("unable to get storage container for disk usage for container %s: %w", container.ID(), err)
	}
	usage, err := driver.ReadWriteDiskUsage(storageContainer.LayerID)
	if err != nil {
		return writableLayer, fmt.Errorf("unable to get disk usage for container %s: %w", container.ID(), err)
	}
	writableLayer.UsedBytes = &types.UInt64Value{Value: uint64(usage.Size)}
	writableLayer.InodesUsed = &types.UInt64Value{Value: uint64(usage.InodeCount)}

	return writableLayer, nil
}

// StatsForSandbox returns the stats for the given sandbox.
func (ss *StatsServer) StatsForSandbox(sb *sandbox.Sandbox) *types.PodSandboxStats {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.statsForSandbox(sb)
}

// StatsForSandboxes returns the stats for the given list of sandboxes.
func (ss *StatsServer) StatsForSandboxes(sboxes []*sandbox.Sandbox) []*types.PodSandboxStats {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	stats := make([]*types.PodSandboxStats, 0, len(sboxes))
	for _, sb := range sboxes {
		if stat := ss.statsForSandbox(sb); stat != nil {
			stats = append(stats, stat)
		}
	}
	return stats
}

// statsForSandbox is an internal, non-locking version of StatsForSandbox
// that returns (and occasionally gathers) the stats for the given sandbox.
func (ss *StatsServer) statsForSandbox(sb *sandbox.Sandbox) *types.PodSandboxStats {
	if ss.collectionPeriod == 0 {
		return ss.updateSandbox(sb)
	}
	sboxStat, ok := ss.sboxStats[sb.ID()]
	if ok {
		return sboxStat
	}
	// Cache miss, try again
	return ss.updateSandbox(sb)
}

// RemoveStatsForSandbox removes the saved entry for the specified sandbox
// to prevent the map from always growing.
func (ss *StatsServer) RemoveStatsForSandbox(sb *sandbox.Sandbox) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	delete(ss.sboxStats, sb.ID())
}

// StatsForContainer returns the stats for the given container.
func (ss *StatsServer) StatsForContainer(c *oci.Container, sb *sandbox.Sandbox) *types.ContainerStats {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.statsForContainer(c, sb)
}

// StatsForContainers returns the stats for the given list of containers.
func (ss *StatsServer) StatsForContainers(ctrs []*oci.Container) []*types.ContainerStats {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	stats := make([]*types.ContainerStats, 0, len(ctrs))
	for _, c := range ctrs {
		sb := ss.GetSandbox(c.Sandbox())
		if sb == nil {
			logrus.Errorf("Unexpectedly failed to get sandbox %s for container %s", c.Sandbox(), c.ID())
			continue
		}

		if stat := ss.statsForContainer(c, sb); stat != nil {
			stats = append(stats, stat)
		}
	}
	return stats
}

// statsForContainer is an internal, non-locking version of StatsForContainer
// that returns (and occasionally gathers) the stats for the given container.
func (ss *StatsServer) statsForContainer(c *oci.Container, sb *sandbox.Sandbox) *types.ContainerStats {
	if ss.collectionPeriod == 0 {
		return ss.updateContainerStats(c, sb)
	}
	ctrStat, ok := ss.ctrStats[c.ID()]
	if ok {
		return ctrStat
	}
	return ss.updateContainerStats(c, sb)
}

// RemoveStatsForContainer removes the saved entry for the specified container
// to prevent the map from always growing.
func (ss *StatsServer) RemoveStatsForContainer(c *oci.Container) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	delete(ss.ctrStats, c.ID())
}

// Shutdown tells the updateLoop to stop updating.
func (ss *StatsServer) Shutdown() {
	if ss.alreadyShutdown {
		return
	}
	close(ss.shutdown)
	ss.alreadyShutdown = true
}

// MetricsForPodSandbox returns the metrics for the given sandbox pod/container.
func (ss *StatsServer) MetricsForPodSandbox(sb *sandbox.Sandbox) *SandboxMetrics {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.metricsForPodSandbox(sb)
}

// MetricsForPodSandboxList returns the metrics for the given list of sandboxes.
func (ss *StatsServer) MetricsForPodSandboxList(sboxes []*sandbox.Sandbox) []*SandboxMetrics {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	metricsList := make([]*SandboxMetrics, 0, len(sboxes))
	for _, sb := range sboxes {
		if metrics := ss.metricsForPodSandbox(sb); metrics != nil {
			metricsList = append(metricsList, metrics)
		}
	}
	return metricsList
}

// RemoveMetricsForPodSandbox removes the saved entry for the specified sandbox
// to prevent the map from always growing.
func (ss *StatsServer) RemoveMetricsForPodSandbox(sb *sandbox.Sandbox) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	delete(ss.sboxMetrics, sb.ID())
}
