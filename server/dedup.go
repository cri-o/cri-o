package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/docker/go-units"
	"go.podman.io/storage"
	"golang.org/x/sys/unix"

	"github.com/cri-o/cri-o/internal/log"
)

// RunDedup runs a single storage deduplication pass using reflinks.
// It calls store.Dedup with SHA256 hashing and logs the result.
func RunDedup(ctx context.Context, store storage.Store) error {
	log.Infof(ctx, "Starting storage deduplication")

	result, err := store.Dedup(storage.DedupArgs{
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

	if result.Deduped == 0 {
		log.Infof(ctx, "Storage deduplication complete: no savings")
	} else {
		log.Infof(ctx, "Storage deduplication complete: %s saved", units.BytesSize(float64(result.Deduped)))
	}

	return nil
}
