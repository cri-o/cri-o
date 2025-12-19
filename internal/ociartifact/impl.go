package ociartifact

import (
	"context"
	"fmt"
	"io"

	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/pkg/shortnames"
	"go.podman.io/image/v5/types"
)

type Impl interface {
	ParseNormalizedNamed(string) (reference.Named, error)
	DockerNewReference(reference.Named) (types.ImageReference, error)
	DockerReferenceString(types.ImageReference) string
	DockerReferenceName(types.ImageReference) string
	LayoutNewReference(string, string) (types.ImageReference, error)
	NewImageSource(context.Context, types.ImageReference, *types.SystemContext) (types.ImageSource, error)
	CloseImageSource(types.ImageSource) error
	LayerInfos(manifest.Manifest) []manifest.LayerInfo
	GetBlob(context.Context, types.ImageSource, types.BlobInfo, types.BlobInfoCache) (io.ReadCloser, int64, error)
	ReadAll(io.Reader) ([]byte, error)
	CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]reference.Named, error)
}

// defaultImpl is the default implementation for the OCI artifact handling.
type defaultImpl struct{}

func (*defaultImpl) ParseNormalizedNamed(s string) (reference.Named, error) {
	return reference.ParseNormalizedNamed(s)
}

func (*defaultImpl) DockerNewReference(ref reference.Named) (types.ImageReference, error) {
	return docker.NewReference(ref)
}

func (*defaultImpl) DockerReferenceString(ref types.ImageReference) string {
	return ref.DockerReference().String()
}

func (*defaultImpl) DockerReferenceName(ref types.ImageReference) string {
	return ref.DockerReference().Name()
}

func (*defaultImpl) CloseImageSource(src types.ImageSource) error {
	return src.Close()
}

func (*defaultImpl) LayoutNewReference(dir, imageRef string) (types.ImageReference, error) {
	return layout.NewReference(dir, imageRef)
}

func (*defaultImpl) NewImageSource(ctx context.Context, ref types.ImageReference, sys *types.SystemContext) (types.ImageSource, error) {
	return ref.NewImageSource(ctx, sys)
}

func (*defaultImpl) LayerInfos(m manifest.Manifest) []manifest.LayerInfo {
	return m.LayerInfos()
}

//nolint:gocritic // it's intentional to stick to the original implementation
func (*defaultImpl) GetBlob(ctx context.Context, src types.ImageSource, bi types.BlobInfo, bic types.BlobInfoCache) (io.ReadCloser, int64, error) {
	return src.GetBlob(ctx, bi, bic)
}

func (*defaultImpl) ReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// CandidatesForPotentiallyShortImageName resolves locally an artifact name into a set of fully-qualified image names (domain/repo/image:tag|@digest).
// It will only return an empty slice if err != nil.
func (d *defaultImpl) CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]reference.Named, error) {
	if shortnames.IsShortName(imageName) {
		return nil, fmt.Errorf("artifact %q must be a fully-qualified reference; short names and unqualified-search-registries are not supported for artifacts", imageName)
	}

	namedRef, err := reference.ParseNormalizedNamed(imageName)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact name %q: %w", imageName, err)
	}

	return []reference.Named{reference.TagNameOnly(namedRef)}, nil
}
