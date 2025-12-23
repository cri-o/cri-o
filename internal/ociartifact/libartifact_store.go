package ociartifact

import (
	"context"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	libartStore "go.podman.io/common/pkg/libartifact/store"
)

// LibartifactStore is the interface to mock libartifact store.
// It should have the same methods signature as libartifact.ArtifactStore.
type LibartifactStore interface {
	Remove(ctx context.Context, ref libartStore.ArtifactStoreReference) (*digest.Digest, error)
	List(ctx context.Context) (libartifact.ArtifactList, error)
	Pull(ctx context.Context, ref libartStore.ArtifactReference, opts libimage.CopyOptions) (digest.Digest, error)
	Inspect(ctx context.Context, ref libartStore.ArtifactStoreReference) (*libartifact.Artifact, error)
}
