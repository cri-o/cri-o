package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/types"

	"github.com/cri-o/cri-o/internal/blobcache"
	"github.com/cri-o/cri-o/internal/log"
)

const (
	// blobCacheTimeout is the maximum time to wait for blob caching to complete.
	// This prevents goroutine leaks if the cache operation hangs.
	blobCacheTimeout = 30 * time.Second
)

// blobCachingReference wraps an ImageReference to intercept NewImageDestination.
type blobCachingReference struct {
	types.ImageReference

	cache      *blobcache.BlobCache
	registry   string
	repository string
}

// NewBlobCachingReference wraps an image reference with blob caching.
func NewBlobCachingReference(ref types.ImageReference, cache *blobcache.BlobCache, registry, repository string) types.ImageReference {
	if cache == nil {
		return ref
	}

	return &blobCachingReference{
		ImageReference: ref,
		cache:          cache,
		registry:       registry,
		repository:     repository,
	}
}

// NewImageDestination wraps the destination with blob caching.
func (r *blobCachingReference) NewImageDestination(ctx context.Context, sys *types.SystemContext) (types.ImageDestination, error) {
	dest, err := r.ImageReference.NewImageDestination(ctx, sys)
	if err != nil {
		return nil, err
	}

	return NewBlobCachingDestination(dest, r.cache, r.registry, r.repository), nil
}

// blobCachingDestination wraps an image destination to cache compressed blobs.
type blobCachingDestination struct {
	types.ImageDestination

	cache      *blobcache.BlobCache
	registry   string
	repository string
}

// NewBlobCachingDestination wraps a destination with blob caching.
func NewBlobCachingDestination(dest types.ImageDestination, cache *blobcache.BlobCache, registry, repository string) types.ImageDestination {
	if cache == nil {
		return dest
	}

	return &blobCachingDestination{
		ImageDestination: dest,
		cache:            cache,
		registry:         registry,
		repository:       repository,
	}
}

// PutBlob intercepts blob writes to cache compressed layer blobs.
//
//nolint:gocritic // hugeParam: blobInfo signature required by types.ImageDestination interface
func (b *blobCachingDestination) PutBlob(ctx context.Context, stream io.Reader, blobInfo types.BlobInfo, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	if isConfig {
		return b.ImageDestination.PutBlob(ctx, stream, blobInfo, cache, isConfig)
	}

	// If blob has no digest, just pass through (we need the digest to store it).
	if blobInfo.Digest == "" {
		return b.ImageDestination.PutBlob(ctx, stream, blobInfo, cache, isConfig)
	}

	// Create a TeeReader to capture the blob while it's being written.
	pr, pw := io.Pipe()
	teeReader := io.TeeReader(stream, pw)

	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)
		defer pr.Close()

		// Use a separate context for the cache operation to avoid premature cancellation.
		// This ensures the goroutine can complete even if the parent context is cancelled.
		cacheCtx, cancel := context.WithTimeout(context.Background(), blobCacheTimeout)
		defer cancel()

		err := b.cache.StoreBlob(cacheCtx, blobInfo.Digest, pr, b.registry, b.repository)
		if err != nil {
			log.Warnf(ctx, "Failed to cache blob %s: %v", blobInfo.Digest, err)

			errChan <- err
		} else {
			log.Debugf(ctx, "Cached blob %s for %s/%s", blobInfo.Digest, b.registry, b.repository)

			errChan <- nil
		}
	}()

	resultInfo, err := b.ImageDestination.PutBlob(ctx, teeReader, blobInfo, cache, isConfig)

	// Close the pipe writer first to signal the caching goroutine that no more data is coming.
	// This ensures the goroutine can complete its work.
	pw.Close()

	if err != nil {
		return resultInfo, err
	}

	// Wait for blob caching with a timeout instead of relying on context cancellation.
	// This prevents goroutine leaks if the cache operation takes too long or hangs.
	select {
	case cacheErr := <-errChan:
		if cacheErr != nil {
			log.Warnf(ctx, "Blob caching failed but pull succeeded: %v", cacheErr)
		}
	case <-time.After(blobCacheTimeout):
		log.Warnf(ctx, "Timeout waiting for blob cache after %v", blobCacheTimeout)
	}

	return resultInfo, nil
}

// ParseRegistryAndRepository extracts registry and repository from an image reference
// using the official docker reference parser for robustness.
func ParseRegistryAndRepository(imageRef string) (registry, repository string, err error) {
	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return "", "", fmt.Errorf("parsing image reference %q: %w", imageRef, err)
	}

	domain := reference.Domain(named)
	path := reference.Path(named)

	return domain, path, nil
}
