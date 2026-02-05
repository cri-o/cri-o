package blobcache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	digest "github.com/opencontainers/go-digest"

	"github.com/cri-o/cri-o/internal/log"
)

var (
	// ErrEmptyDirectory is returned when the cache directory is empty.
	ErrEmptyDirectory = errors.New("blob cache directory cannot be empty")
	// ErrRelativePath is returned when the cache directory is not absolute.
	ErrRelativePath = errors.New("blob cache path must be absolute")
	// ErrInvalidDigest is returned when a digest is invalid.
	ErrInvalidDigest = errors.New("invalid digest")
	// ErrDigestMismatch is returned when blob content doesn't match its digest.
	ErrDigestMismatch = errors.New("digest mismatch")
)

// BlobCache manages compressed layer blobs for P2P distribution.
// It is thread-safe for concurrent access within a single process.
type BlobCache struct {
	rootDir      string
	metadataPath string
	metadata     *Metadata
	mu           sync.RWMutex
}

// Metadata is the root structure of metadata.json.
type Metadata struct {
	Blobs map[string]BlobInfo `json:"blobs"`
}

// BlobInfo contains metadata for a single blob.
type BlobInfo struct {
	Digest       string    `json:"digest"`
	Size         int64     `json:"size"`
	Sources      []Source  `json:"sources"`
	LastAccessed time.Time `json:"lastAccessed,omitzero"`
	CreatedAt    time.Time `json:"createdAt,omitzero"`
}

// Source identifies where a blob came from.
type Source struct {
	Registry   string `json:"registry"`
	Repository string `json:"repository"`
}

// New creates a new blob cache at the specified directory.
func New(ctx context.Context, rootDir string) (*BlobCache, error) {
	if rootDir == "" {
		return nil, ErrEmptyDirectory
	}

	// Security: path must be absolute to prevent traversal.
	if !filepath.IsAbs(rootDir) {
		return nil, fmt.Errorf("%w: %s", ErrRelativePath, rootDir)
	}

	// Security: resolve symlinks and use real path.
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating cache directory: %w", err)
	}

	realPath, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return nil, fmt.Errorf("resolving cache path: %w", err)
	}

	rootDir = realPath

	// Create blobs base directory (algorithm subdirs created on-demand).
	blobsDir := filepath.Join(rootDir, "blobs")
	if err := os.MkdirAll(blobsDir, 0o700); err != nil {
		return nil, fmt.Errorf("creating blobs directory: %w", err)
	}

	// Load or create metadata.
	metadataPath := filepath.Join(rootDir, "metadata.json")

	metadata, err := loadMetadata(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("loading metadata: %w", err)
	}

	log.Infof(ctx, "Initialized blob cache at %s", rootDir)

	return &BlobCache{
		rootDir:      rootDir,
		metadataPath: metadataPath,
		metadata:     metadata,
	}, nil
}

// StoreBlob saves a compressed blob and records its source.
// It verifies the content matches the provided digest.
//
// This method is idempotent: storing the same blob (same digest) multiple times
// just updates the source list and access timestamp - it doesn't duplicate data.
//
// The reader is always fully consumed, even on error, to support TeeReader usage.
func (bc *BlobCache) StoreBlob(ctx context.Context, dgst digest.Digest, reader io.Reader, registry, repository string) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if err := dgst.Validate(); err != nil {
		drainReader(ctx, reader)

		return fmt.Errorf("%w: %w", ErrInvalidDigest, err)
	}

	blobPath := bc.blobPath(dgst)

	if _, err := os.Stat(blobPath); err == nil {
		drainReader(ctx, reader)

		return bc.addSourceLocked(ctx, dgst, registry, repository)
	}

	blobDir := filepath.Dir(blobPath)
	if err := os.MkdirAll(blobDir, 0o700); err != nil {
		drainReader(ctx, reader)

		return fmt.Errorf("creating blob directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(blobDir, ".blob-*.tmp")
	if err != nil {
		drainReader(ctx, reader)

		return fmt.Errorf("creating temp file: %w", err)
	}

	tmpPath := tmpFile.Name()
	success := false

	defer func() {
		if !success {
			tmpFile.Close()
			os.Remove(tmpPath)
		}
	}()

	// Use a MultiWriter to calculate digest while writing.
	verifier := dgst.Verifier()
	writer := io.MultiWriter(tmpFile, verifier)

	written, err := io.Copy(writer, reader)
	if err != nil {
		return fmt.Errorf("writing blob: %w", err)
	}

	// Verify digest.
	if !verifier.Verified() {
		return fmt.Errorf("%w: expected %s", ErrDigestMismatch, dgst)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, blobPath); err != nil {
		return fmt.Errorf("renaming blob: %w", err)
	}

	success = true

	// Update metadata with current time.
	now := time.Now()
	bc.metadata.Blobs[dgst.String()] = BlobInfo{
		Digest:       dgst.String(),
		Size:         written,
		LastAccessed: now,
		CreatedAt:    now,
		Sources: []Source{{
			Registry:   registry,
			Repository: repository,
		}},
	}

	log.Debugf(ctx, "Cached blob %s (%d bytes) for %s/%s", dgst, written, registry, repository)

	return bc.saveMetadata()
}

// blobPath returns the internal path for a blob.
func (bc *BlobCache) blobPath(dgst digest.Digest) string {
	return filepath.Join(bc.rootDir, "blobs", dgst.Algorithm().String(), dgst.Encoded())
}

func (bc *BlobCache) addSourceLocked(ctx context.Context, dgst digest.Digest, registry, repository string) error {
	digestStr := dgst.String()

	info, exists := bc.metadata.Blobs[digestStr]
	if !exists {
		// Blob file exists but metadata doesn't - reconstruct it.
		blobPath := bc.blobPath(dgst)

		stat, err := os.Stat(blobPath)
		if err != nil {
			log.Warnf(ctx, "Failed to stat blob %s at %s: %v", dgst, blobPath, err)

			return fmt.Errorf("stat blob %s: %w", dgst, err)
		}

		info = BlobInfo{
			Digest:       digestStr,
			Size:         stat.Size(),
			CreatedAt:    stat.ModTime(),
			LastAccessed: time.Now(),
		}
	}

	// Update last accessed time.
	info.LastAccessed = time.Now()

	// Check if source already exists.
	for _, s := range info.Sources {
		if s.Registry == registry && s.Repository == repository {
			bc.metadata.Blobs[digestStr] = info

			return bc.saveMetadata()
		}
	}

	info.Sources = append(info.Sources, Source{Registry: registry, Repository: repository})
	bc.metadata.Blobs[digestStr] = info

	log.Debugf(ctx, "Added source %s/%s to blob %s", registry, repository, dgst)

	return bc.saveMetadata()
}

// saveMetadata persists the metadata to disk. Caller must hold bc.mu.
func (bc *BlobCache) saveMetadata() error {
	data, err := json.MarshalIndent(bc.metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling metadata: %w", err)
	}

	// Atomic write.
	tmpFile, err := os.CreateTemp(filepath.Dir(bc.metadataPath), ".metadata-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp metadata: %w", err)
	}

	tmpPath := tmpFile.Name()
	success := false

	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()

		return fmt.Errorf("writing metadata: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp metadata: %w", err)
	}

	if err := os.Rename(tmpPath, bc.metadataPath); err != nil {
		return err
	}

	success = true

	return nil
}

// GCStats contains statistics from a garbage collection operation.
type GCStats struct {
	BlobsRemoved int
	BytesFreed   int64
}

// GarbageCollect removes blobs that are no longer referenced by any image.
// It returns statistics about the GC operation.
func (bc *BlobCache) GarbageCollect(ctx context.Context, referencedDigests map[string]bool) (GCStats, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	var (
		toRemove   []string
		bytesFreed int64
		removed    int
	)

	for digestStr := range bc.metadata.Blobs {
		if !referencedDigests[digestStr] {
			toRemove = append(toRemove, digestStr)
		}
	}

	for _, digestStr := range toRemove {
		dgst, err := digest.Parse(digestStr)
		if err != nil {
			log.Warnf(ctx, "Invalid digest in cache metadata: %s", digestStr)
			delete(bc.metadata.Blobs, digestStr)

			continue
		}

		info := bc.metadata.Blobs[digestStr]

		if err := os.Remove(bc.blobPath(dgst)); err != nil && !os.IsNotExist(err) {
			log.Warnf(ctx, "Failed to remove cached blob %s: %v", digestStr, err)

			continue
		}

		delete(bc.metadata.Blobs, digestStr)

		bytesFreed += info.Size
		removed++
	}

	if removed > 0 {
		if err := bc.saveMetadata(); err != nil {
			return GCStats{}, fmt.Errorf("saving metadata after GC: %w", err)
		}

		log.Infof(ctx, "Blob cache GC: removed %d blobs, freed %d bytes", removed, bytesFreed)
	}

	return GCStats{
		BlobsRemoved: removed,
		BytesFreed:   bytesFreed,
	}, nil
}

func loadMetadata(path string) (*Metadata, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Metadata{Blobs: make(map[string]BlobInfo)}, nil
	}

	if err != nil {
		return nil, err
	}

	var m Metadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}

	if m.Blobs == nil {
		m.Blobs = make(map[string]BlobInfo)
	}

	return &m, nil
}

func drainReader(ctx context.Context, reader io.Reader) {
	if _, err := io.Copy(io.Discard, reader); err != nil {
		log.Warnf(ctx, "Failed to drain reader: %v", err)
	}
}
