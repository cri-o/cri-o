package ociartifact

import (
	"context"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	"go.podman.io/image/v5/types"
)

// LibartifactStore abstracts the libartifact storage operations so that
// the Store can be tested with mock implementations.
type LibartifactStore interface {
	Remove(ctx context.Context, asr libartifact.ArtifactStoreReference) (*digest.Digest, error)
	List(ctx context.Context) (libartifact.ArtifactList, error)
	Pull(ctx context.Context, ref libartifact.ArtifactReference, opts libimage.CopyOptions) (digest.Digest, error)
	Inspect(ctx context.Context, asr libartifact.ArtifactStoreReference) (*libartifact.Artifact, error)
	SystemContext() *types.SystemContext
}

// artifactStore wraps *libartifact.ArtifactStore to satisfy
// the LibartifactStore interface by exposing the SystemContext field
// as a method.
type artifactStore struct {
	*libartifact.ArtifactStore
}

func (s *artifactStore) SystemContext() *types.SystemContext {
	return s.ArtifactStore.SystemContext
}
