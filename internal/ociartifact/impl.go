package ociartifact

import (
	"context"
	"fmt"

	"github.com/opencontainers/go-digest"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
)

// Impl is the main interface for OCI artifact manifest operations.
// It abstracts manifest retrieval and platform resolution so that the
// Store can be tested with mock implementations.
type Impl interface {
	// ChooseInstance selects the platform-specific manifest digest from a
	// multi-architecture manifest list based on the provided system context
	// (OS, architecture, variant).
	ChooseInstance(manifest.List, *types.SystemContext) (digest.Digest, error)

	// GetManifestFromRef fetches the raw manifest bytes and its MIME type
	// from the given image reference. If instanceDigest is non-nil, it
	// retrieves that specific manifest instance rather than the root manifest.
	GetManifestFromRef(context.Context, types.ImageReference, *types.SystemContext, *digest.Digest) ([]byte, string, error)
}

// defaultImpl is the default implementation for the OCI artifact handling.
type defaultImpl struct{}

func (*defaultImpl) ChooseInstance(list manifest.List, sys *types.SystemContext) (digest.Digest, error) {
	return list.ChooseInstance(sys)
}

func (*defaultImpl) GetManifestFromRef(ctx context.Context, ref types.ImageReference, sys *types.SystemContext, instanceDigest *digest.Digest) (manifestBlob []byte, mimeType string, err error) {
	src, err := ref.NewImageSource(ctx, sys)
	if err != nil {
		return nil, "", fmt.Errorf("create image source: %w", err)
	}
	defer src.Close()

	return src.GetManifest(ctx, instanceDigest)
}
