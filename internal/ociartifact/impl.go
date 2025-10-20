package ociartifact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/opencontainers/go-digest"
	"go.podman.io/common/libimage"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/image"
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
	GetManifest(context.Context, types.ImageSource, *digest.Digest) ([]byte, string, error)
	LayerInfos(manifest.Manifest) []manifest.LayerInfo
	GetBlob(context.Context, types.ImageSource, types.BlobInfo, types.BlobInfoCache) (io.ReadCloser, int64, error)
	ReadAll(io.Reader) ([]byte, error)
	OCI1FromManifest([]byte) (*manifest.OCI1, error)
	ToJSON(any) ([]byte, error)
	ManifestFromBlob([]byte, string) (manifest.Manifest, error)
	ListFromBlob([]byte, string) (manifest.List, error)
	ManifestConfigMediaType(manifest.Manifest) string
	NewCopier(*libimage.CopyOptions, *types.SystemContext) (*libimage.Copier, error)
	Copy(context.Context, *libimage.Copier, types.ImageReference, types.ImageReference) ([]byte, error)
	CloseCopier(*libimage.Copier) error
	List(string) ([]layout.ListResult, error)
	DeleteImage(context.Context, types.ImageReference, *types.SystemContext) error
	CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]reference.Named, error)
	ChooseInstance(manifest.List, *types.SystemContext) (digest.Digest, error)
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

func (*defaultImpl) ToJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (*defaultImpl) ManifestFromBlob(manblob []byte, mt string) (manifest.Manifest, error) {
	return manifest.FromBlob(manblob, mt)
}

func (*defaultImpl) ListFromBlob(manblob []byte, mt string) (manifest.List, error) {
	return manifest.ListFromBlob(manblob, mt)
}

func (*defaultImpl) ManifestConfigMediaType(parsedManifest manifest.Manifest) string {
	return parsedManifest.ConfigInfo().MediaType
}

func (*defaultImpl) NewCopier(options *libimage.CopyOptions, sc *types.SystemContext) (*libimage.Copier, error) {
	return libimage.NewCopier(options, sc)
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

// CandidatesForPotentiallyShortImageName resolves locally an artifact name into a set of fully-qualified image names (domain/repo/image:tag|@digest).
// It will only return an empty slice if err != nil.
func (d *defaultImpl) CandidatesForPotentiallyShortImageName(systemContext *types.SystemContext, imageName string) ([]reference.Named, error) {
	// Always resolve unqualified names to all candidates. We should use a more secure mode once we settle on a shortname alias table.
	sc := types.SystemContext{}
	if systemContext != nil {
		sc = *systemContext // A shallow copy
	}

	resolved, err := shortnames.ResolveLocally(&sc, imageName)
	if err != nil {
		// Error is not very clear in this context, and unfortunately is also not a variable.
		if strings.Contains(err.Error(), "short-name resolution enforced but cannot prompt without a TTY") {
			return nil, fmt.Errorf("short name mode is enforcing, but image name %s returns ambiguous list", imageName)
		}

		return nil, err
	}

	return resolved, nil
}

func (d *defaultImpl) ChooseInstance(manifestList manifest.List, systemContext *types.SystemContext) (digest.Digest, error) {
	return manifestList.ChooseInstance(systemContext)
}
