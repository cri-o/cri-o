package ociartifact

import (
	"context"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	"go.podman.io/image/v5/types"
)

type LibartifactStore interface {
	Remove(ctx context.Context, asr libartifact.ArtifactStoreReference) (*digest.Digest, error)
	List(ctx context.Context) (libartifact.ArtifactList, error)
	Pull(ctx context.Context, ref libartifact.ArtifactReference, opts libimage.CopyOptions) (digest.Digest, error)
	Inspect(ctx context.Context, asr libartifact.ArtifactStoreReference) (*libartifact.Artifact, error)
	// SystemContext returns the internal system context
	SystemContext() *types.SystemContext
}

type RealLibartifactStore struct {
	*libartifact.ArtifactStore
}

func (r RealLibartifactStore) SystemContext() *types.SystemContext {
	return r.ArtifactStore.SystemContext
}
