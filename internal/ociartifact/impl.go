package ociartifact

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/containers/common/libimage"
	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/image"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
)

type Impl interface {
	ParseNormalizedNamed(string) (reference.Named, error)
	DockerNewReference(reference.Named) (types.ImageReference, error)
	DockerReferenceString(types.ImageReference) string
	LayoutNewReference(string, string) (types.ImageReference, error)
	NewImageSource(context.Context, types.ImageReference, *types.SystemContext) (types.ImageSource, error)
	CloseImageSource(types.ImageSource) error
	GetManifest(context.Context, types.ImageSource, *digest.Digest) ([]byte, string, error)
	LayerInfos(manifest.Manifest) []manifest.LayerInfo
	GetBlob(context.Context, types.ImageSource, types.BlobInfo, types.BlobInfoCache) (io.ReadCloser, int64, error)
	ReadAll(io.Reader) ([]byte, error)
	OCI1FromManifest([]byte) (*manifest.OCI1, error)
	MarshalJSON(any) ([]byte, error)
	ManifestFromBlob([]byte, string) (manifest.Manifest, error)
	ManifestConfigMediaType(manifest.Manifest) string
	NewCopier(*libimage.CopyOptions, *types.SystemContext, *types.ImageReference) (*libimage.Copier, error)
	Copy(context.Context, *libimage.Copier, types.ImageReference, types.ImageReference) ([]byte, error)
	CloseCopier(*libimage.Copier) error
	List(string) ([]layout.ListResult, error)
	DeleteImage(context.Context, types.ImageReference, *types.SystemContext) error
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

func (*defaultImpl) CloseImageSource(src types.ImageSource) error {
	return src.Close()
}

func (*defaultImpl) LayoutNewReference(dir, image string) (types.ImageReference, error) {
	return layout.NewReference(dir, image)
}

func (*defaultImpl) NewImageSource(ctx context.Context, ref types.ImageReference, sys *types.SystemContext) (types.ImageSource, error) {
	return ref.NewImageSource(ctx, sys)
}

func (*defaultImpl) GetManifest(ctx context.Context, src types.ImageSource, instanceDigest *digest.Digest) (bytes []byte, mediaType string, err error) {
	return image.UnparsedInstance(src, instanceDigest).Manifest(ctx)
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

func (*defaultImpl) OCI1FromManifest(manifestBlob []byte) (*manifest.OCI1, error) {
	return manifest.OCI1FromManifest(manifestBlob)
}

func (*defaultImpl) MarshalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (*defaultImpl) ManifestFromBlob(manblob []byte, mt string) (manifest.Manifest, error) {
	return manifest.FromBlob(manblob, mt)
}

func (*defaultImpl) ManifestConfigMediaType(parsedManifest manifest.Manifest) string {
	return parsedManifest.ConfigInfo().MediaType
}

//nolint:gocritic // intentionally pointer
func (*defaultImpl) NewCopier(options *libimage.CopyOptions, sc *types.SystemContext, reportResolvedReference *types.ImageReference) (*libimage.Copier, error) {
	return libimage.NewCopier(options, sc, reportResolvedReference)
}

func (d *defaultImpl) Copy(ctx context.Context, copier *libimage.Copier, source, destination types.ImageReference) ([]byte, error) {
	return copier.Copy(ctx, source, destination)
}

func (d *defaultImpl) CloseCopier(copier *libimage.Copier) error {
	return copier.Close()
}

func (d *defaultImpl) List(dir string) ([]layout.ListResult, error) {
	result, err := layout.List(dir)
	// If the dir is empty, it returns os.ErrNotExist, but should return an empty list.
	// This happens because the dir isn't initialized as an oci-layout dir until something is pulled.
	if errors.Is(err, os.ErrNotExist) {
		return []layout.ListResult{}, nil
	}

	return result, err
}

func (d *defaultImpl) DeleteImage(ctx context.Context, ref types.ImageReference, sys *types.SystemContext) error {
	return ref.DeleteImage(ctx, sys)
}
