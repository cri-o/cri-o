package statsmgr

import (
	"sync"

	"github.com/cri-o/cri-o/server/cri/types"
)

// legacyStatsManager is an implementation of the StatsManager interface
// that covers the legacy functionality: check the disk stats with every
// ContainerStats{,List} call.
// The main purpose of this implementation is to prevent duplicated
// DiskStats caches if the client is also keeping a cache (as the Kubelet does
// by default with cadvisor as of the time of writing this).
// Users should transition to using cachedStatsManager and change their
// clients to not keep their own cache.
type legacyStatsManager struct {
	pathMap map[string]string
	sync.RWMutex
}

// newLegacyStatsManager creates a new legacyStatsManager.
func newLegacyStatsManager() *legacyStatsManager {
	return &legacyStatsManager{
		pathMap: make(map[string]string),
	}
}

// AddID adds a container to the set of containers that legacyStatsManager
// will update when asked to.
func (sm *legacyStatsManager) AddID(id, path string) {
	sm.Lock()
	sm.pathMap[id] = path
	sm.Unlock()
}

// RemoveID removes the container with ID `id` from the set of contaienrs
// that will have their stats updated when requested.
func (sm *legacyStatsManager) RemoveID(id string) {
	sm.Lock()
	defer sm.Unlock()
	delete(sm.pathMap, id)
}

// UpdateWithDiskStats updates a ContainerStats object to have the DiskStats of the containers
// legacyStatsManager is keeping track of.
func (sm *legacyStatsManager) UpdateWithDiskStats(criStats []*types.ContainerStats) error {
	return updateWithDiskStats(criStats, sm.statsForID)
}

// statsForID is an internal function that returns the computed DirSize and InodeCount.
func (sm *legacyStatsManager) statsForID(id string) (dirSize, inodeCount uint64, _ error) {
	sm.RLock()
	defer sm.RUnlock()
	path, ok := sm.pathMap[id]
	// the most likely situation this occurs is
	// we've just added the container, and the stats object hasn't been populated yet
	// it's an unlikely race, and the client will be okay not knowing the stats
	// for this new container for one request
	if !ok {
		return 0, 0, nil
	}

	dirSize, inodeCount, err := GetDiskUsageStats(path)
	if err != nil {
		return 0, 0, err
	}

	return dirSize, inodeCount, nil
}

// Shutdown is a noop for legacyStatsManager, as there is no routine keeping the cache.
func (*legacyStatsManager) Shutdown() {}
