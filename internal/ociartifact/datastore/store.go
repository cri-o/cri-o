package datastore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/common/libimage"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/image"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/pkg/blobinfocache"
	"go.podman.io/image/v5/types"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/ociartifact"
)

// defaultMaxArtifactSize is the default size per artifact data.
const defaultMaxArtifactSize = 1 * 1024 * 1024 // 1 MiB

// ArtifactData separates the artifact metadata from the actual content.
type ArtifactData struct {
	data []byte
}

// Data returns the data of the artifact.
func (a *ArtifactData) Data() []byte {
	return a.data
}

// Store handles pulling artifact data and reading blobs.
// It embeds ociartifact.Store and adds data-pulling capabilities.
type Store struct {
	*ociartifact.Store

	systemContext *types.SystemContext
	impl          Impl
}

// New creates a new OCI artifact data store.
func New(rootPath string, systemContext *types.SystemContext, allowImages bool) (*Store, error) {
	// The datastore only handles artifacts pulled into the main store.
	// Additional read-only stores are not threaded through here (we pass
	// nil for additionalPaths) since the datastore is used for in-memory
	// artifact data managed by the main CRI-O lifecycle.
	ociStore, err := ociartifact.NewStore(rootPath, nil, systemContext, nil, allowImages)
	if err != nil {
		return nil, fmt.Errorf("create OCI artifact store: %w", err)
	}

	return &Store{
		Store:         ociStore,
		systemContext: systemContext,
		impl:          &defaultImpl{},
	}, nil
}

// PullOptions can be used to customize the pull behavior.
type PullOptions struct {
	// EnforceConfigMediaType can be set to enforce a specific manifest config
	// media type.
	EnforceConfigMediaType string

	// MaxSize is the maximum size of the artifact to be allowed to stay
	// in-memory. This is only useful when requesting the artifact data using
	// PullData.
	// Will be set to a default of 1MiB if not specified (zero) or below zero.
	MaxSize uint64

	// CopyOptions are the copy options passed down to libimage.
	CopyOptions *libimage.CopyOptions
}

// PullData downloads the artifact into the local storage and returns its data.
// Returns ociartifact.ErrNotFound if the artifact is not available.
func (s *Store) PullData(ctx context.Context, ref string, opts *PullOptions) ([]ArtifactData, error) {
	opts = sanitizeOptions(opts)

	log.Infof(ctx, "Pulling OCI artifact from ref: %s", ref)

	dockerRef, err := s.getImageReference(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to get image reference: %w", err)
	}

	manifestDigest, err := s.Pull(ctx, dockerRef, opts.CopyOptions)
	if err != nil {
		return nil, fmt.Errorf("pull artifact: %w", err)
	}

	artifactData, err := s.artifactData(ctx, manifestDigest.Encoded(), opts.MaxSize)
	if err != nil {
		return nil, fmt.Errorf("get artifact data: %w", err)
	}

	return artifactData, nil
}

func sanitizeOptions(opts *PullOptions) *PullOptions {
	if opts == nil {
		opts = &PullOptions{}
	}

	if opts.MaxSize == 0 {
		opts.MaxSize = defaultMaxArtifactSize
	}

	if opts.CopyOptions == nil {
		opts.CopyOptions = &libimage.CopyOptions{}
	}

	return opts
}

func (s *Store) artifactData(ctx context.Context, nameOrDigest string, maxArtifactSize uint64) (res []ArtifactData, err error) {
	artifact, nameIsDigest, err := s.getByNameOrDigest(ctx, nameOrDigest)
	if err != nil {
		return nil, fmt.Errorf("get artifact by name or digest: %w", err)
	}

	if nameIsDigest {
		nameOrDigest = artifact.Reference()
	}

	imageReference, err := s.impl.LayoutNewReference(artifact.RootPath(), nameOrDigest)
	if err != nil {
		return nil, fmt.Errorf("create new reference: %w", err)
	}

	imageSource, err := s.impl.NewImageSource(ctx, imageReference, s.systemContext)
	if err != nil {
		return nil, fmt.Errorf("build image source: %w", err)
	}

	defer func() {
		if err := s.impl.CloseImageSource(imageSource); err != nil {
			log.Warnf(ctx, "Unable to close image source: %v", err)
		}
	}()

	readSize := uint64(0)

	layerInfos := s.impl.LayerInfos(artifact.Manifest)
	for i := range layerInfos {
		layer := &layerInfos[i]

		layerBytes, err := s.readBlob(ctx, imageSource, layer, maxArtifactSize)
		if err != nil {
			return nil, fmt.Errorf("read artifact blob: %w", err)
		}

		readSize += uint64(len(layerBytes))
		if readSize > maxArtifactSize {
			return nil, fmt.Errorf("exceeded maximum allowed artifact size of %d bytes", maxArtifactSize)
		}

		res = append(res, ArtifactData{data: layerBytes})
	}

	return res, nil
}

func (s *Store) readBlob(ctx context.Context, src types.ImageSource, layer *manifest.LayerInfo, maxArtifactSize uint64) ([]byte, error) {
	bic := blobinfocache.DefaultCache(s.systemContext)

	rc, size, err := s.impl.GetBlob(ctx, src, types.BlobInfo{Digest: layer.Digest}, bic)
	if err != nil {
		return nil, fmt.Errorf("get artifact blob: %w", err)
	}
	defer rc.Close()

	if size != -1 && size > int64(maxArtifactSize)+1 {
		return nil, fmt.Errorf("exceeded maximum allowed size of %d bytes for a single layer", maxArtifactSize)
	}

	limitedReader := io.LimitReader(rc, int64(maxArtifactSize+1))

	layerBytes, err := s.impl.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read from limit reader: %w", err)
	}

	if err := verifyDigest(layer, layerBytes); err != nil {
		return nil, fmt.Errorf("verify digest of layer: %w", err)
	}

	return layerBytes, nil
}

func verifyDigest(layer *manifest.LayerInfo, layerBytes []byte) error {
	expectedDigest := layer.Digest
	if err := expectedDigest.Validate(); err != nil {
		return fmt.Errorf("invalid digest %q: %w", expectedDigest, err)
	}

	digestAlgorithm := expectedDigest.Algorithm()
	digester := digestAlgorithm.Digester()

	hash := digester.Hash()
	hash.Write(layerBytes)
	sum := hash.Sum(nil)

	layerBytesHex := hex.EncodeToString(sum)
	if layerBytesHex != layer.Digest.Hex() {
		return fmt.Errorf(
			"%s mismatch between real layer bytes (%s) and manifest descriptor (%s)",
			digestAlgorithm, layerBytesHex, layer.Digest.Hex(),
		)
	}

	return nil
}

// getByNameOrDigest retrieves an artifact by its name or digest.
// Returns the artifact, a boolean indicating whether strRef was a digest (true) or name (false),
// and an error if the artifact could not be found.
// Returns ociartifact.ErrNotFound if no matching artifact exists.
func (s *Store) getByNameOrDigest(ctx context.Context, strRef string) (*ociartifact.Artifact, bool, error) {
	if strRef == "" {
		return nil, false, errors.New("empty name or digest")
	}

	artifacts, err := s.List(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("list artifacts: %w", err)
	}

	// if strRef is a just digest or short digest
	if idx := slices.IndexFunc(artifacts, func(a *ociartifact.Artifact) bool { return strings.HasPrefix(a.Digest().Encoded(), strRef) }); len(strRef) >= 3 && idx != -1 {
		return artifacts[idx], true, nil
	}

	// if strRef is named reference
	candidates, err := s.impl.CandidatesForPotentiallyShortImageName(s.systemContext, strRef)
	if err != nil {
		// If there are no artifacts in the store, return ErrNotFound regardless of name validation errors
		// This maintains backward compatibility where invalid names simply weren't found
		if len(artifacts) == 0 {
			return nil, false, fmt.Errorf("%w with name or digest of: %s", ociartifact.ErrNotFound, strRef)
		}

		return nil, false, fmt.Errorf("get candidates for potentially short image name: %w", err)
	}

	for _, candidate := range candidates {
		for _, artifact := range artifacts {
			if candidate.String() == artifact.Reference() || candidate.String() == artifact.CanonicalName() {
				return artifact, false, nil
			}
		}
	}

	return nil, false, fmt.Errorf("%w with name or digest of: %s", ociartifact.ErrNotFound, strRef)
}

func (s *Store) getImageReference(nameOrDigest string) (types.ImageReference, error) {
	name, err := s.impl.ParseNormalizedNamed(nameOrDigest)
	if err != nil {
		return nil, fmt.Errorf("parse image name: %w", err)
	}

	name = reference.TagNameOnly(name) // make sure to add ":latest" if needed

	ref, err := s.impl.DockerNewReference(name)
	if err != nil {
		return nil, fmt.Errorf("create docker reference: %w", err)
	}

	return ref, nil
}

// PullConfig returns the pull an image configuration defined by the manifest digest.
// There is no such config attached to OCI artifacts, but this function can
// be used to retrieve an image's config without pulling the whole data.
// This is useful for our support of runtimes that manage image pulls on their own:
// cri-o still needs to get the config, but should not pull the data layers.
func (s *Store) PullConfig(ctx context.Context, nameOrDigest string, opts *PullOptions) (*specs.Image, error) {
	artifact, nameIsDigest, err := s.getByNameOrDigest(ctx, nameOrDigest)
	if err != nil {
		return nil, fmt.Errorf("get artifact by name or digest: %w", err)
	}

	if nameIsDigest {
		nameOrDigest = artifact.Reference()
	}

	// get the ImageSource for the image
	imageReference, err := s.impl.LayoutNewReference(artifact.RootPath(), nameOrDigest)
	if err != nil {
		return nil, fmt.Errorf("create new reference: %w", err)
	}

	imageSource, err := s.impl.NewImageSource(ctx, imageReference, s.systemContext)
	if err != nil {
		return nil, fmt.Errorf("build image source: %w", err)
	}

	defer func() {
		if err := s.impl.CloseImageSource(imageSource); err != nil {
			log.Warnf(ctx, "Unable to close image source: %v", err)
		}
	}()

	unparsedToplevel := image.UnparsedInstance(imageSource, nil)

	topManifest, topMIMEType, err := unparsedToplevel.Manifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	unparsedInstance := unparsedToplevel

	if manifest.MIMETypeIsMultiImage(topMIMEType) {
		// This is a manifest list. We need to choose a single instance to work with.
		manifestList, err := manifest.ListFromBlob(topManifest, topMIMEType)
		if err != nil {
			return nil, fmt.Errorf("parsing primary manifest as list: %w", err)
		}

		instanceDigest, err := manifestList.ChooseInstance(s.systemContext)
		if err != nil {
			return nil, fmt.Errorf("choosing an image from manifest list: %w", err)
		}

		unparsedInstance = image.UnparsedInstance(imageSource, &instanceDigest)
	}

	sourcedImage, err := image.FromUnparsedImage(ctx, s.systemContext, unparsedInstance)
	if err != nil {
		return nil, fmt.Errorf("getting sourced image from unparsed image: %w", err)
	}

	return sourcedImage.OCIConfig(ctx)
}
