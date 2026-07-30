package datastore

import (
	"context"
	"io"
	"os"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	libartTypes "go.podman.io/common/pkg/libartifact/types"
)

// Impl is the interface for the implementation.
type Impl interface {
	NewArtifactReference(string) (libartifact.ArtifactReference, error)
	ReadFile(string, int64) ([]byte, error)
}

// LibartifactStore is the interface for the underlying artifact store.
type LibartifactStore interface {
	Pull(ctx context.Context, ref libartifact.ArtifactReference, opts libimage.CopyOptions) (digest.Digest, error)
	BlobMountPaths(ctx context.Context, asr libartifact.ArtifactStoreReference, opts *libartTypes.BlobMountPathOptions) ([]libartTypes.BlobMountPath, error)
}

// defaultImpl is the default implementation for the OCI artifact handling.
type defaultImpl struct{}

func (*defaultImpl) NewArtifactReference(s string) (libartifact.ArtifactReference, error) {
	return libartifact.NewArtifactReference(s)
}

func (*defaultImpl) ReadFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(io.LimitReader(f, limit))
}
