package ociartifact

import (
	"context"
	"io"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
)

type Impl interface {
	ParseNormalizedNamed(string) (reference.Named, error)
	NewReference(reference.Named) (types.ImageReference, error)
	NewImageSource(context.Context, types.ImageReference, *types.SystemContext) (types.ImageSource, error)
	GetManifest(context.Context, types.ImageSource, *digest.Digest) ([]byte, string, error)
	ManifestFromBlob([]byte, string) (manifest.Manifest, error)
	ManifestConfigInfo(manifest.Manifest) types.BlobInfo
	LayerInfos(manifest.Manifest) []manifest.LayerInfo
	GetBlob(context.Context, types.ImageSource, types.BlobInfo, types.BlobInfoCache) (io.ReadCloser, int64, error)
	ReadAll(io.Reader) ([]byte, error)
}

// defaultImpl is the default implementation for the OCI artifact handling.
type defaultImpl struct{}

func (*defaultImpl) ParseNormalizedNamed(s string) (reference.Named, error) {
	return reference.ParseNormalizedNamed(s)
}

func (*defaultImpl) NewReference(ref reference.Named) (types.ImageReference, error) {
	return docker.NewReference(ref)
}

func (*defaultImpl) NewImageSource(ctx context.Context, ref types.ImageReference, sys *types.SystemContext) (types.ImageSource, error) {
	return ref.NewImageSource(ctx, sys)
}

func (*defaultImpl) GetManifest(ctx context.Context, src types.ImageSource, instanceDigest *digest.Digest) (bytes []byte, mediaType string, err error) {
	return src.GetManifest(ctx, instanceDigest)
}

func (*defaultImpl) ManifestFromBlob(manblob []byte, mt string) (manifest.Manifest, error) {
	return manifest.FromBlob(manblob, mt)
}

func (*defaultImpl) ManifestConfigInfo(m manifest.Manifest) types.BlobInfo {
	return m.ConfigInfo()
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
