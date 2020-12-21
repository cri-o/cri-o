package statsmgr

import (
	"fmt"
	"sync"
	"time"

	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/pkg/errors"
)

// SyncPeriod is the amount of time between
// refreshing the disk stats.
// It is based on the Kubelet's defaultHousekeepingInterval
// (https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/cadvisor/cadvisor_linux.go#L65).
var SyncPeriod time.Duration = 10 * time.Second

// cachedStatsManager is an implementation of StatsManager
// that results in fewer disk reads.
// Specifically, it caches the InodeCount and DirSize for
// each container ID it is in charge of.
// It refreshes this cache every `SyncPeriod`.
type cachedStatsManager struct {
	statsMap map[string]*managedDiskStats
	done     chan struct{}
	sync.RWMutex
}

// newCachedStatsManager creates a new CachedStatsManager,
// and starts the updateLoop in a new goroutine
func newCachedStatsManager() *cachedStatsManager {
	sm := &cachedStatsManager{
		statsMap: make(map[string]*managedDiskStats),
		done:     make(chan struct{}, 1),
	}
	go sm.updateLoop()

	return sm
}

// managedDiskStats is an internal structure that holds the cached stats,
// along with a lock to keep its update atomic, and the path of the rootFs of the container.
type managedDiskStats struct {
	path       string
	errorCount int
	lastError  error
	InodeCount uint64
	DirSize    uint64
	sync.RWMutex
}

// updateLoop runs every `SyncPeriod`, or until Shutdown() is called.
// It updates all of the container stats that are currently tracked.
func (sm *cachedStatsManager) updateLoop() {
	for {
		sm.RLock()
		for _, stats := range sm.statsMap {
			go stats.updateStats()
		}
		sm.RUnlock()

		select {
		case <-time.After(SyncPeriod):
		case <-sm.done:
			return
		}
	}
}

// UpdateWithDiskStats updates the ContainerStats object with the disk stats
// for every container the cachedStatsManager currently watches.
func (sm *cachedStatsManager) UpdateWithDiskStats(criStats []*types.ContainerStats) error {
	return updateWithDiskStats(criStats, sm.statsForID)
}

// statsForID is an internal function that returns the cached DirSize and InodeCount.
func (sm *cachedStatsManager) statsForID(id string) (dirSize, inodeCount uint64, _ error) {
	sm.RLock()
	defer sm.RUnlock()
	managedStat, ok := sm.statsMap[id]
	// the most likely situation this occurs is
	// we've just added the container, and the stats object hasn't been populated yet
	// it's an unlikely race, and the client will be okay not knowing the stats
	// for this new container for one request
	if !ok {
		return 0, 0, nil
	}
	managedStat.RLock()
	defer managedStat.RUnlock()

	// errors should be reported
	if managedStat.errorCount > 0 {
		// If we don't attempt to update the stats,
		// then these requests will error continuously until they're
		// re-updated. However, we don't want to waste resources
		// updating stats that will never succeed.
		if managedStat.errorCount < 5 {
			go managedStat.updateStats()
		}
		return 0, 0, errors.Errorf("finding disk stats has failed the following number of times: %d. Last error: %v", managedStat.errorCount, managedStat.lastError)
	}

	return managedStat.DirSize, managedStat.InodeCount, nil
}

// AddID adds a container to the set of containers that cachedStatsManager watches.
// It will initialize the disk stats, and save the ID and path for cache updates.
func (sm *cachedStatsManager) AddID(id, path string) {
	mds := &managedDiskStats{
		path: path,
	}

	sm.Lock()
	sm.statsMap[id] = mds
	sm.Unlock()

	mds.updateStats()
}

// updateStats updates the DirSize and InodeCount for this container's managedDiskStats.
func (mds *managedDiskStats) updateStats() {
	mds.Lock()
	defer mds.Unlock()
	dirSize, inodeCount, err := GetDiskUsageStats(mds.path)
	if err != nil {
		fmt.Println("hello")
		mds.errorCount++
		mds.lastError = err
		return
	}

	// reset the error count
	mds.errorCount = 0
	mds.lastError = nil

	mds.DirSize = dirSize
	mds.InodeCount = inodeCount
}

// RemoveID causes the cachedStatsManager to no longer
// track the disk usage of this container ID.
func (sm *cachedStatsManager) RemoveID(id string) {
	sm.Lock()
	defer sm.Unlock()
	delete(sm.statsMap, id)
}

// Shutdown stops the updateLoop.
func (sm *cachedStatsManager) Shutdown() {
	sm.done <- struct{}{}
}
