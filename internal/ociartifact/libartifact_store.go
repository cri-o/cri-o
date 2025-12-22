package ociartifact

import (
	"context"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	libartStore "go.podman.io/common/pkg/libartifact/store"
	"go.podman.io/image/v5/types"
)

type LibartifactStore interface {
	// Remove an artifact from the local artifact store.
	Remove(ctx context.Context, name string) (*digest.Digest, error)

	// List artifacts in the local store.
	List(ctx context.Context) (libartifact.ArtifactList, error)

	// Pull an artifact from an image registry to a local store.
	Pull(ctx context.Context, name string, opts libimage.CopyOptions) (digest.Digest, error)

	// SystemContext returns the internal system context
	SystemContext() *types.SystemContext
}

type RealLibartifactStore struct {
	*libartStore.ArtifactStore
}

func (r RealLibartifactStore) SystemContext() *types.SystemContext {
	return r.ArtifactStore.SystemContext
}
