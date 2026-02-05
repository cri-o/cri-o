package ociartifact

import (
	"context"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
)

type LibartifactStore interface {
	Remove(ctx context.Context, asr libartifact.ArtifactStoreReference) (*digest.Digest, error)
	List(ctx context.Context) (libartifact.ArtifactList, error)
	Pull(ctx context.Context, ref libartifact.ArtifactReference, opts libimage.CopyOptions) (digest.Digest, error)
	Inspect(ctx context.Context, asr libartifact.ArtifactStoreReference) (*libartifact.Artifact, error)
}
