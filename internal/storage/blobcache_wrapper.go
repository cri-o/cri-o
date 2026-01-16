package storage

import (
	"context"
	"fmt"
	"io"

	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/types"

	"github.com/cri-o/cri-o/internal/blobcache"
	"github.com/cri-o/cri-o/internal/log"
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
// The cache write happens synchronously in parallel with the storage write via TeeReader.
// This ensures the cache is always in a known state (success or failure) when the pull completes.
//
//nolint:gocritic // hugeParam: blobInfo signature required by types.ImageDestination interface
func (b *blobCachingDestination) PutBlob(ctx context.Context, stream io.Reader, blobInfo types.BlobInfo, cache types.BlobInfoCache, isConfig bool) (types.BlobInfo, error) {
	// Skip caching for config blobs - only cache layer blobs for P2P distribution.
	if isConfig {
		return b.ImageDestination.PutBlob(ctx, stream, blobInfo, cache, isConfig)
	}

	// If blob has no digest, just pass through (we need the digest to store it).
	if blobInfo.Digest == "" {
		return b.ImageDestination.PutBlob(ctx, stream, blobInfo, cache, isConfig)
	}

	// Create a TeeReader to write to both storage and cache simultaneously.
	// This ensures both destinations receive identical bytes from the same stream,
	// eliminating TOCTOU risks where tag movement could cause divergence.
	pr, pw := io.Pipe()
	teeReader := io.TeeReader(stream, pw)

	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)
		defer pr.Close()

		// Use the same context as the pull operation.
		// This ensures proper cancellation propagation - if the pull is cancelled,
		// the cache write is also cancelled, preventing orphaned goroutines.
		err := b.cache.StoreBlob(ctx, blobInfo.Digest, pr, b.registry, b.repository)
		errChan <- err
	}()

	// Write to storage - this drives data through the TeeReader to both destinations.
	resultInfo, err := b.ImageDestination.PutBlob(ctx, teeReader, blobInfo, cache, isConfig)

	// Close the pipe writer to signal EOF to the cache goroutine.
	// This allows the cache write to complete even if storage finished first.
	pw.Close()

	if err != nil {
		// Storage write failed - cache goroutine will also fail (broken pipe or context cancelled).
		// No need to wait for it.
		return resultInfo, err
	}

	// Wait synchronously for cache write to complete.
	// The pipe naturally synchronizes the writes - by the time we reach here,
	// the cache write is almost done (just finishing the final flush).
	// This typically adds only 1-10ms to the pull time.
	cacheErr := <-errChan
	if cacheErr != nil {
		// Log warning but don't fail the pull - cache is best-effort for P2P optimization.
		// The image is safely stored; cache failure just means this blob won't be available for P2P.
		log.Warnf(ctx, "Blob %s stored successfully but cache write failed: %v", blobInfo.Digest, cacheErr)
	} else {
		log.Debugf(ctx, "Cached blob %s for %s/%s", blobInfo.Digest, b.registry, b.repository)
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
