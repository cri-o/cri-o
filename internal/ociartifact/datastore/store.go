package datastore

import (
	"context"
	"fmt"
	"path/filepath"

	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	libartTypes "go.podman.io/common/pkg/libartifact/types"
	"go.podman.io/image/v5/types"

	"github.com/cri-o/cri-o/internal/log"
)

// defaultMaxArtifactSize is the default size per artifact data.
const defaultMaxArtifactSize = 1 * 1024 * 1024 // 1 MiB

// ArtifactData separates the artifact metadata from the actual content.
type ArtifactData struct {
	data []byte
}

// Data returns the data of the artifact.
func (a *ArtifactData) Data() []byte {
	return a.data
}

// Store handles pulling artifact data and reading blobs using the podman
// libartifact store directly.
type Store struct {
	store LibartifactStore
	impl  Impl
}

// New creates a new OCI artifact data store.
func New(rootPath string, systemContext *types.SystemContext) (*Store, error) {
	storePath := filepath.Join(rootPath, "artifacts")

	artStore, err := libartifact.NewArtifactStore(storePath, systemContext)
	if err != nil {
		return nil, fmt.Errorf("create artifact store: %w", err)
	}

	return &Store{
		store: artStore,
		impl:  &defaultImpl{},
	}, nil
}

// PullOptions can be used to customize the pull behavior.
type PullOptions struct {
	// MaxSize is the maximum size of the artifact to be allowed to stay
	// in-memory. This is only useful when requesting the artifact data using
	// PullData.
	// Will be set to a default of 1MiB if not specified (zero) or below zero.
	MaxSize uint64

	// CopyOptions are the copy options passed down to libimage.
	CopyOptions *libimage.CopyOptions
}

// PullData downloads the artifact into the local storage and returns its data.
func (s *Store) PullData(ctx context.Context, ref string, opts *PullOptions) ([]ArtifactData, error) {
	opts = sanitizeOptions(opts)

	log.Infof(ctx, "Pulling OCI artifact from ref: %s", ref)

	artRef, err := s.impl.NewArtifactReference(ref)
	if err != nil {
		return nil, fmt.Errorf("create artifact reference: %w", err)
	}

	if _, err := s.store.Pull(ctx, artRef, *opts.CopyOptions); err != nil {
		return nil, fmt.Errorf("pull artifact: %w", err)
	}

	blobPaths, err := s.store.BlobMountPaths(ctx, artRef.ToArtifactStoreReference(), &libartTypes.BlobMountPathOptions{})
	if err != nil {
		return nil, fmt.Errorf("get blob mount paths: %w", err)
	}

	return s.readBlobData(blobPaths, opts.MaxSize)
}

func (s *Store) readBlobData(blobPaths []libartTypes.BlobMountPath, maxSize uint64) ([]ArtifactData, error) {
	var res []ArtifactData

	totalSize := uint64(0)

	for _, bp := range blobPaths {
		remaining := int64(maxSize - totalSize)

		data, err := s.impl.ReadFile(bp.SourcePath, remaining+1)
		if err != nil {
			return nil, fmt.Errorf("read blob file: %w", err)
		}

		totalSize += uint64(len(data))
		if totalSize > maxSize {
			return nil, fmt.Errorf("exceeded maximum allowed artifact size of %d bytes", maxSize)
		}

		res = append(res, ArtifactData{data: data})
	}

	return res, nil
}

func sanitizeOptions(opts *PullOptions) *PullOptions {
	if opts == nil {
		opts = &PullOptions{}
	}

	if opts.MaxSize == 0 {
		opts.MaxSize = defaultMaxArtifactSize
	}

	if opts.CopyOptions == nil {
		opts.CopyOptions = &libimage.CopyOptions{}
	}

	return opts
}
