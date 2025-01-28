package ociartifact

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache"
	"github.com/containers/image/v5/types"

	"github.com/cri-o/cri-o/internal/log"
)

// OCIArtifact is the main structure of this package.
type OCIArtifact struct {
	impl Impl
}

// New returns a new OCI artifact implementation.
func New() *OCIArtifact {
	return &OCIArtifact{
		impl: &defaultImpl{},
	}
}

// Artifact can be used to manage OCI artifacts.
type Artifact struct {
	// Data is the actual artifact content.
	Data []byte

	// Path is the file representation of the artifact on disk. Can be empty if
	// the cache is not used.
	Path string
}

// PullOptions can be used to customize the pull behavior.
type PullOptions struct {
	// SystemContext is the context used for pull.
	SystemContext *types.SystemContext

	// CachePath is the path to the used artifact cache. The cache can be
	// disabled if the value is an empty string.
	CachePath string

	// CacheEntryMaxAge is the maximum age cache entries can have before
	// getting removed.
	//
	// Defaults to 3 days.
	// Can be set to a negative value for disabling the removal.
	//
	// Note: Items only get removed on calling Pull(), which means they can be
	// technically older on disk.
	CacheEntryMaxAge time.Duration

	// EnforceConfigMediaType can be set to enforce a specific manifest config media type.
	EnforceConfigMediaType string

	// MaxSize is the maximum size of the artifact to be allowed to get pulled.
	// Will be set to a default of 1MiB if not specified (zero) or below zero.
	MaxSize int
}

const (
	// defaultMaxArtifactSize is the default maximum artifact size.
	defaultMaxArtifactSize = 1024 * 1024 // 1 MiB

	// defaultCacheEntryMaxAge is the default max age for a cache entry item.
	defaultCacheEntryMaxAge = 3 * 24 * time.Hour // 3 days
)

// Pull downloads the artifact content by using the provided image name and the specified options.
func (o *OCIArtifact) Pull(ctx context.Context, img string, opts *PullOptions) (*Artifact, error) {
	log.Infof(ctx, "Pulling OCI artifact from ref: %s", img)

	// Use default pull options
	if opts == nil {
		opts = &PullOptions{}
	}

	name, err := o.impl.ParseNormalizedNamed(img)
	if err != nil {
		return nil, fmt.Errorf("parse image name: %w", err)
	}

	name = reference.TagNameOnly(name) // make sure to add ":latest" if needed

	ref, err := o.impl.NewReference(name)
	if err != nil {
		return nil, fmt.Errorf("create docker reference: %w", err)
	}

	src, err := o.impl.NewImageSource(ctx, ref, opts.SystemContext)
	if err != nil {
		return nil, fmt.Errorf("build image source: %w", err)
	}

	manifestBytes, mimeType, err := o.impl.GetManifest(ctx, src, nil)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	parsedManifest, err := o.impl.ManifestFromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if opts.EnforceConfigMediaType != "" && o.impl.ManifestConfigInfo(parsedManifest).MediaType != opts.EnforceConfigMediaType {
		return nil, fmt.Errorf(
			"wrong config media type %q, requires %q",
			o.impl.ManifestConfigInfo(parsedManifest).MediaType,
			opts.EnforceConfigMediaType,
		)
	}

	layers := o.impl.LayerInfos(parsedManifest)
	if len(layers) < 1 {
		return nil, errors.New("artifact needs at least one layer")
	}

	// Just supporting one layer here. This can be later enhanced by extending
	// the PullOptions.
	layer := &layers[0]

	useCache := o.prepareCache(ctx, opts)
	if artifact := o.tryLookupCache(ctx, useCache, opts, layer); artifact != nil {
		return artifact, nil
	}

	bic := blobinfocache.DefaultCache(opts.SystemContext)

	rc, size, err := o.impl.GetBlob(ctx, src, layer.BlobInfo, bic)
	if err != nil {
		return nil, fmt.Errorf("get layer blob: %w", err)
	}

	defer rc.Close()

	maxArtifactSize := defaultMaxArtifactSize
	if opts.MaxSize > 0 {
		maxArtifactSize = opts.MaxSize
	}

	if size != -1 && size > int64(maxArtifactSize)+1 {
		return nil, fmt.Errorf("exceeded maximum allowed size of %d bytes", maxArtifactSize)
	}

	layerBytes, err := o.readLimit(rc, maxArtifactSize)
	if err != nil {
		return nil, fmt.Errorf("read with limit: %w", err)
	}

	if err := verifyDigest(layer, layerBytes); err != nil {
		return nil, fmt.Errorf("verify digest of layer: %w", err)
	}

	keyPath := o.tryWriteCache(ctx, useCache, opts, layer, layerBytes)

	return &Artifact{
		Data: layerBytes,
		Path: keyPath,
	}, nil
}

func (o *OCIArtifact) prepareCache(ctx context.Context, opts *PullOptions) (useCache bool) {
	if opts.CachePath == "" {
		return false
	}

	if err := o.impl.MkdirAll(opts.CachePath, 0o700); err != nil {
		log.Errorf(ctx, "Unable to create cache path: %v", err)

		return false
	}

	entries, err := o.impl.ReadDir(opts.CachePath)
	if err != nil {
		log.Errorf(ctx, "Unable to read cache path: %v", err)

		return false
	}

	maxAge := opts.CacheEntryMaxAge
	if opts.CacheEntryMaxAge == 0 {
		maxAge = defaultCacheEntryMaxAge
	}

	now := time.Now()

	if maxAge > 0 {
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				log.Errorf(ctx, "Unable to get artifact layer info for %q: %v", entry.Name(), err)

				continue
			}

			if diff := now.Sub(info.ModTime()); diff > maxAge {
				p := filepath.Join(opts.CachePath, info.Name())
				log.Infof(ctx, "Removing old artifact layer %q (age: %v)", p, diff.Round(time.Second))

				if err := o.impl.RemoveAll(p); err != nil {
					log.Errorf(ctx, "Unable to remove artifact layer %q: %v", p, err)
				}
			}
		}
	}

	return true
}

func (o *OCIArtifact) tryLookupCache(ctx context.Context, useCache bool, opts *PullOptions, layer *manifest.LayerInfo) *Artifact {
	if !useCache {
		return nil
	}

	keyPath := getKeyPath(opts, layer)
	tryRemoveKeyPath := false

	if value, err := o.impl.ReadFile(keyPath); err == nil {
		if err := verifyDigest(layer, value); err == nil {
			log.Infof(ctx, "Using cached artifact layer for digest %q", layer.BlobInfo.Digest)

			return &Artifact{
				Data: value,
				Path: keyPath,
			}
		}

		log.Warnf(ctx, "Unable to verify cached artifact layer digest: %v", err)

		tryRemoveKeyPath = true
	} else if !os.IsNotExist(err) {
		log.Warnf(ctx, "Unable to read cached artifact layer: %v", err)

		tryRemoveKeyPath = true
	}

	if tryRemoveKeyPath {
		log.Infof(ctx, "Removing cached artifact layer for digest %q", layer.BlobInfo.Digest)

		if err := o.impl.RemoveAll(keyPath); err != nil {
			log.Warnf(ctx, "Unable to remove artifact from cache: %v", err)
		}
	}

	return nil
}

func (o *OCIArtifact) tryWriteCache(ctx context.Context, useCache bool, opts *PullOptions, layer *manifest.LayerInfo, layerBytes []byte) (keyPath string) {
	if !useCache {
		return ""
	}

	keyPath = getKeyPath(opts, layer)

	if err := o.impl.WriteFile(keyPath, layerBytes, 0o600); err != nil {
		log.Errorf(ctx, "Unable to write artifact layer to cache: %v", err)

		return ""
	}

	return keyPath
}

func getKeyPath(opts *PullOptions, layer *manifest.LayerInfo) string {
	key := string(layer.BlobInfo.Digest)

	return filepath.Join(opts.CachePath, key)
}

func (o *OCIArtifact) readLimit(reader io.Reader, limit int) ([]byte, error) {
	limitedReader := io.LimitReader(reader, int64(limit+1))

	res, err := o.impl.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read from limit reader: %w", err)
	}

	if len(res) > limit {
		return nil, fmt.Errorf("exceeded maximum allowed size of %d bytes", limit)
	}

	return res, nil
}

func verifyDigest(layer *manifest.LayerInfo, layerBytes []byte) error {
	expectedDigest := layer.BlobInfo.Digest
	if err := expectedDigest.Validate(); err != nil {
		return fmt.Errorf("invalid digest %q: %w", expectedDigest, err)
	}

	digestAlgorithm := expectedDigest.Algorithm()
	digester := digestAlgorithm.Digester()

	hash := digester.Hash()
	hash.Write(layerBytes)
	sum := hash.Sum(nil)

	layerBytesHex := hex.EncodeToString(sum)
	if layerBytesHex != layer.BlobInfo.Digest.Hex() {
		return fmt.Errorf(
			"%s mismatch between real layer bytes (%s) and manifest descriptor (%s)",
			digestAlgorithm, layerBytesHex, layer.BlobInfo.Digest.Hex(),
		)
	}

	return nil
}
