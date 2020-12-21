package statsmgr

import (
	"os"
	"path/filepath"

	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// CachedStatsManagerType is the string used to configure CRI-O to use
// the CachedStatsManager
const CachedStatsManagerType = "cached"

// StatsManager is the interface for checking for disk stats of containers
type StatsManager interface {
	AddID(string, string)
	RemoveID(string)
	UpdateWithDiskStats([]*types.ContainerStats) error
	Shutdown()
}

// New creates a new StatsManager based on the server configuration
func New(statsManager string) StatsManager {
	if statsManager == CachedStatsManagerType {
		return newCachedStatsManager()
	}
	return newLegacyStatsManager()
}

// updateWithDiskStats is an internal helper function that covers shared behavior between the different implementations
// of StatsManager.UpdateWithDiskStats.
// It takes a functor `statsForIDFunc` that should return the usedBytes and inodesUsed for that particular container ID.
func updateWithDiskStats(criStats []*types.ContainerStats, statsForIDFunc func(string) (uint64, uint64, error)) error {
	failedUpdates := make([]string, 0)
	for _, criStat := range criStats {
		// shouldn't happen but good to check
		if criStat == nil {
			return errors.Errorf("ContainerStats object is invalid with a nil entry")
		}
		if criStat.Attributes == nil {
			return errors.Errorf("ContainerStats object is invalid with a nil entry")
		}
		if criStat.WritableLayer == nil {
			criStat.WritableLayer = &types.FilesystemUsage{}
		}

		usedBytes, inodesUsed, err := statsForIDFunc(criStat.Attributes.ID)
		if err != nil {
			logrus.Errorf("Failed to update disk stats for container %s: %v", criStat.Attributes.ID, err)
			failedUpdates = append(failedUpdates, criStat.Attributes.ID)
			continue
		}
		criStat.WritableLayer.UsedBytes = &types.UInt64Value{Value: usedBytes}
		criStat.WritableLayer.InodesUsed = &types.UInt64Value{Value: inodesUsed}
	}
	if len(failedUpdates) > 0 {
		return errors.Errorf("Failed to update disk stats for containers: %v", failedUpdates)
	}
	return nil
}

// GetDiskUsageStats accepts a path to a directory or file
// and returns the number of bytes and inodes used by the path
func GetDiskUsageStats(path string) (dirSize, inodeCount uint64, _ error) {
	if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		// Walk does not follow symbolic links
		if err != nil {
			return err
		}

		dirSize += uint64(info.Size())
		inodeCount++

		return nil
	}); err != nil {
		return 0, 0, err
	}

	return dirSize, inodeCount, nil
}
