package server

import (
	"context"
	"errors"
	"fmt"
	"path"

	"go.podman.io/storage"
	"golang.org/x/sys/unix"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/utils"
)

// RunDedup runs a single storage deduplication pass using reflinks.
// It calls store.Dedup with SHA256 hashing. If showPhysicalUsage is true,
// measures real physical disk usage before and after dedup using FIEMAP (Linux only).
func RunDedup(ctx context.Context, store storage.Store, showPhysicalUsage bool) error {
	var beforeUsage uint64

	// Measure physical usage before dedup
	if showPhysicalUsage {
		log.Infof(ctx, "Measuring physical disk usage before deduplication...")

		usage, err := measurePhysicalUsage(ctx, store)
		if err != nil {
			log.Warnf(ctx, "Failed to measure initial disk usage: %v", err)
		} else {
			beforeUsage = usage
			log.Infof(ctx, "Physical disk usage before dedup: %s", formatBytes(beforeUsage))
		}
	}

	log.Infof(ctx, "Starting storage deduplication (this may take several minutes)...")

	_, err := store.Dedup(storage.DedupArgs{
		Options: storage.DedupOptions{
			HashMethod: storage.DedupHashSHA256,
		},
	})
	if err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EOPNOTSUPP) {
			return fmt.Errorf("storage deduplication not supported on current filesystem: %w", err)
		}

		return fmt.Errorf("storage deduplication failed: %w", err)
	}

	log.Infof(ctx, "Storage deduplication complete")

	// Measure physical usage after dedup and show savings
	if showPhysicalUsage && beforeUsage > 0 {
		log.Infof(ctx, "Measuring physical disk usage after deduplication...")

		afterUsage, err := measurePhysicalUsage(ctx, store)
		if err != nil {
			log.Warnf(ctx, "Failed to measure final disk usage: %v", err)
		} else {
			log.Infof(ctx, "Physical disk usage after dedup: %s", formatBytes(afterUsage))

			if beforeUsage > afterUsage {
				saved := beforeUsage - afterUsage
				pct := float64(saved) / float64(beforeUsage) * 100
				log.Infof(ctx, "Space saved by deduplication: %s (%.1f%%)", formatBytes(saved), pct)
			} else {
				log.Infof(ctx, "No space savings detected (blocks may already be shared)")
			}

			// Show detailed breakdown (reuses afterUsage, only measures standard once)
			if err := reportPhysicalUsage(ctx, store, afterUsage); err != nil {
				log.Warnf(ctx, "Failed to report detailed physical disk usage: %v", err)
			}
		}
	}

	return nil
}

func measurePhysicalUsage(_ context.Context, store storage.Store) (uint64, error) {
	rootPath := store.GraphRoot()
	imagePath := store.ImageStore()
	storageDriver := store.GraphDriverName()

	var totalReal uint64

	overlayPath := path.Join(rootPath, storageDriver)

	realBytes, _, err := utils.GetRealPhysicalUsage(overlayPath)
	if err != nil {
		return 0, fmt.Errorf("failed to get real usage for %s: %w", overlayPath, err)
	}

	totalReal += realBytes

	layersPath := path.Join(rootPath, storageDriver+"-layers")

	realBytes, _, err = utils.GetRealPhysicalUsage(layersPath)
	if err == nil {
		totalReal += realBytes
	}

	if imagePath != "" {
		imageOverlayPath := path.Join(imagePath, storageDriver)

		realBytes, _, err = utils.GetRealPhysicalUsage(imageOverlayPath)
		if err != nil {
			return 0, fmt.Errorf("failed to get real usage for %s: %w", imageOverlayPath, err)
		}

		totalReal += realBytes

		imageLayersPath := path.Join(imagePath, storageDriver+"-layers")

		realBytes, _, err = utils.GetRealPhysicalUsage(imageLayersPath)
		if err == nil {
			totalReal += realBytes
		}
	}

	return totalReal, nil
}

func reportPhysicalUsage(ctx context.Context, store storage.Store, totalRealUsage uint64) error {
	rootPath := store.GraphRoot()
	imagePath := store.ImageStore()
	storageDriver := store.GraphDriverName()

	log.Infof(ctx, "=== Detailed Physical Disk Usage (FIEMAP - Reflink-Aware) ===")
	log.Infof(ctx, "Storage driver: %s", storageDriver)
	log.Infof(ctx, "Graph root: %s", rootPath)

	var totalStandard uint64

	overlayPath := path.Join(rootPath, storageDriver)
	log.Infof(ctx, "Layer data storage: %s", overlayPath)

	standardBytes, _, err := utils.GetDiskUsageStats(overlayPath)
	if err != nil {
		return fmt.Errorf("failed to get standard usage for %s: %w", overlayPath, err)
	}

	totalStandard += standardBytes

	layersPath := path.Join(rootPath, storageDriver+"-layers")
	log.Infof(ctx, "Layer metadata: %s", layersPath)

	standardBytes, _, err = utils.GetDiskUsageStats(layersPath)
	if err != nil {
		log.Warnf(ctx, "Could not measure %s: %v", layersPath, err)
	} else {
		totalStandard += standardBytes
	}

	if imagePath != "" {
		imageOverlayPath := path.Join(imagePath, storageDriver)
		log.Infof(ctx, "Image layer data storage: %s", imageOverlayPath)

		standardBytes, _, err = utils.GetDiskUsageStats(imageOverlayPath)
		if err != nil {
			return fmt.Errorf("failed to get standard usage for %s: %w", imageOverlayPath, err)
		}

		totalStandard += standardBytes

		imageLayersPath := path.Join(imagePath, storageDriver+"-layers")
		log.Infof(ctx, "Image layer metadata: %s", imageLayersPath)

		standardBytes, _, err = utils.GetDiskUsageStats(imageLayersPath)
		if err != nil {
			log.Warnf(ctx, "Could not measure %s: %v", imageLayersPath, err)
		} else {
			totalStandard += standardBytes
		}
	}

	// Summary
	log.Infof(ctx, "=== Summary ===")

	saved := int64(totalStandard) - int64(totalRealUsage)
	if saved > 0 {
		pct := float64(saved) / float64(totalStandard) * 100
		log.Infof(ctx, "Standard reporting (stat.Blocks): %s", formatBytes(totalStandard))
		log.Infof(ctx, "Real physical usage (FIEMAP):     %s", formatBytes(totalRealUsage))
		log.Infof(ctx, "Shared blocks from reflinks:      %s (%.1f%%)", formatBytes(uint64(saved)), pct)
	} else {
		log.Infof(ctx, "Total physical usage: %s", formatBytes(totalRealUsage))
		log.Infof(ctx, "No shared blocks detected (standard and FIEMAP match)")
	}

	return nil
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
