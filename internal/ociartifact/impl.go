package ociartifact

import (
	"context"
	"fmt"
	"io"
	"strings"

	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/oci/layout"
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
	namedRef, err := reference.ParseNormalizedNamed(imageName)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact name %q: %w", imageName, err)
	}

	domain := reference.Domain(namedRef)

	// Accept as fully-qualified if it has an explicit domain.
	// ParseNormalizedNamed normalizes short names like "nginx" to "docker.io/library/nginx",
	// so we can't just check the domain. We detect explicit domains by checking if the
	// original input contains a "." (like "docker.io" or "quay.io") or multiple "/"
	// (like "docker.io/user/image"). This follows the same logic as splitDockerDomain
	// in go.podman.io/image/v5/docker/reference/normalize.go.
	hasExplicitDomain := domain != "docker.io" ||
		strings.Contains(imageName, ".") || strings.Count(imageName, "/") > 1

	if !hasExplicitDomain {
		return nil, fmt.Errorf("artifact %q must be a fully-qualified reference; short names and unqualified-search-registries are not supported for artifacts", imageName)
	}

	return []reference.Named{reference.TagNameOnly(namedRef)}, nil
}
