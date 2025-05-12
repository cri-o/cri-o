package ociartifact

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	modelSpec "github.com/CloudNativeAI/model-spec/specs-go/v1"
	"github.com/containers/common/libimage"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/pkg/blobinfocache"
	"github.com/containers/image/v5/types"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// defaultMaxArtifactSize is the default size per artifact data.
const defaultMaxArtifactSize = 1 * 1024 * 1024 // 1 MiB

var (
	// ErrIsAnImage is indicating that the artifact is a container image.
	ErrIsAnImage = errors.New("provided artifact is a container image")

	// ErrNotFound is indicating that the artifact could not be found in the storage.
	ErrNotFound = errors.New("no artifact found")
)

// Store is the main structure to build an artifact storage.
type Store struct {
	rootPath      string
	systemContext *types.SystemContext
	impl          Impl
}

// NewStore creates a new OCI artifact store.
func NewStore(rootPath string, systemContext *types.SystemContext) *Store {
	return &Store{
		rootPath:      filepath.Join(rootPath, "artifacts"),
		systemContext: systemContext,
		impl:          &defaultImpl{},
	}
}

type unknownRef struct{}

func (unknownRef) String() string {
	return "unknown"
}

func (u unknownRef) Name() string {
	return u.String()
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
// Returns ErrNotFound if the artifact is not available.
func (s *Store) PullData(ctx context.Context, ref string, opts *PullOptions) ([]ArtifactData, error) {
	opts = sanitizeOptions(opts)

	log.Infof(ctx, "Pulling OCI artifact from ref: %s", ref)

	dockerRef, err := s.getImageReference(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to get image reference: %w", err)
	}

	manifestBytes, err := s.PullManifest(ctx, dockerRef, opts)
	if err != nil {
		return nil, fmt.Errorf("pull artifact: %w", err)
	}

	artifactData, err := s.artifactData(ctx, digest.FromBytes(manifestBytes).Encoded(), opts.MaxSize)
	if err != nil {
		return nil, fmt.Errorf("get artifact data: %w", err)
	}

	return artifactData, nil
}

// PullManifest tries to pull the artifact and returns the manifest bytes if the
// provided reference is a valid OCI artifact.
//
// Returns ErrIsAnImage if the artifact is a container image.
//
// enforceConfigMediaType can be used to allow only a certain config media type.
// copyOptions will be passed down to libimage.
func (s *Store) PullManifest(
	ctx context.Context,
	ref types.ImageReference,
	opts *PullOptions,
) (manifestBytes []byte, err error) {
	opts = sanitizeOptions(opts)
	strRef := s.impl.DockerReferenceString(ref)

	log.Debugf(ctx, "Checking if source reference is an OCI artifact: %v", strRef)

	src, err := s.impl.NewImageSource(ctx, ref, s.systemContext)
	if err != nil {
		return nil, fmt.Errorf("build image source: %w", err)
	}

	defer func() {
		if err := s.impl.CloseImageSource(src); err != nil {
			log.Warnf(ctx, "Unable to close image source: %v", err)
		}
	}()

	manifestBytes, mimeType, err := s.impl.GetManifest(ctx, src, nil)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	log.Debugf(ctx, "Manifest mime type of %s: %s", strRef, mimeType)

	listMimeTypes := []string{
		specs.MediaTypeImageIndex,
		manifest.DockerV2ListMediaType,
	}
	if slices.Contains(listMimeTypes, mimeType) {
		return nil, ErrIsAnImage
	}

	parsedManifest, err := s.impl.ManifestFromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("parse manifest from blob: %w", err)
	}

	mediaType := s.impl.ManifestConfigMediaType(parsedManifest)

	if opts.EnforceConfigMediaType != "" && mediaType != opts.EnforceConfigMediaType {
		return nil, fmt.Errorf("wrong config media type %q, requires %q", mediaType, opts.EnforceConfigMediaType)
	}

	log.Debugf(ctx, "Config media type of %s: %s", strRef, mediaType)

	imageMimeTypes := []string{
		specs.MediaTypeImageManifest,
		manifest.DockerV2Schema2MediaType,
		manifest.DockerV2Schema1SignedMediaType,
	}
	configMediaTypes := []string{
		"", // empty
		specs.MediaTypeImageConfig,
		manifest.DockerV2Schema2ConfigMediaType,
	}

	if slices.Contains(imageMimeTypes, mimeType) && slices.Contains(configMediaTypes, mediaType) {
		ociManifest, err := manifest.OCI1FromManifest(manifestBytes)
		// Unable to parse an OCI manifest, assume an image
		if err != nil {
			return nil, ErrIsAnImage
		}

		// No artifact type set, assume an image
		if ociManifest.ArtifactType == "" {
			return nil, ErrIsAnImage
		}

		log.Debugf(ctx, "Found artifact type: %s", ociManifest.ArtifactType)
	}

	log.Infof(ctx, "Pulling OCI artifact %s with manifest mime type %q and config media type %q", strRef, mimeType, mediaType)

	copier, err := s.impl.NewCopier(opts.CopyOptions, s.systemContext)
	if err != nil {
		return nil, fmt.Errorf("create libimage copier: %w", err)
	}

	dst, err := s.impl.LayoutNewReference(s.rootPath, strRef)
	if err != nil {
		return nil, fmt.Errorf("create destination reference: %w", err)
	}

	if manifestBytes, err = s.impl.Copy(ctx, copier, ref, dst); err != nil {
		return nil, fmt.Errorf("copy artifact: %w", err)
	}

	if err := s.impl.CloseCopier(copier); err != nil {
		return nil, fmt.Errorf("close copier: %w", err)
	}

	return manifestBytes, nil
}

// List creates a slice of all available artifacts.
func (s *Store) List(ctx context.Context) (res []*Artifact, err error) {
	listResult, err := s.impl.List(s.rootPath)
	if err != nil {
		return nil, fmt.Errorf("list store root: %w", err)
	}

	for i := range listResult {
		artifact, err := s.buildArtifact(ctx, &listResult[i])
		if err != nil {
			return nil, fmt.Errorf("build artifact: %w", err)
		}

		res = append(res, artifact)
	}

	return res, nil
}

// Status retrieves the artifact by referencing a name or digest.
// Returns ErrNotFound if the artifact is not available.
func (s *Store) Status(ctx context.Context, nameOrDigest string) (*Artifact, error) {
	artifact, _, err := s.getByNameOrDigest(ctx, nameOrDigest)
	if err != nil {
		return nil, fmt.Errorf("get artifact by name or digest: %w", err)
	}

	return artifact, nil
}

// Remove deletes a name or digest from the artifact store.
// Returns ErrNotFound if the artifact is not available.
func (s *Store) Remove(ctx context.Context, nameOrDigest string) error {
	artifact, nameIsDigest, err := s.getByNameOrDigest(ctx, nameOrDigest)
	if err != nil {
		return fmt.Errorf("get artifact by name or digest: %w", err)
	}

	if nameIsDigest {
		nameOrDigest = artifact.Reference()
	}

	imageReference, err := s.impl.LayoutNewReference(s.rootPath, nameOrDigest)
	if err != nil {
		return fmt.Errorf("create new reference: %w", err)
	}

	if err := s.impl.DeleteImage(ctx, imageReference, s.systemContext); err != nil {
		return fmt.Errorf("delete artifact: %w", err)
	}

	return nil
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

func (s *Store) buildArtifact(ctx context.Context, item *layout.ListResult) (*Artifact, error) {
	ref := item.Reference

	rawSource, err := s.impl.NewImageSource(ctx, ref, s.systemContext)
	if err != nil {
		return nil, fmt.Errorf("create new image source: %w", err)
	}

	defer func() {
		if err := s.impl.CloseImageSource(rawSource); err != nil {
			log.Warnf(ctx, "Unable to close image source: %v", err)
		}
	}()

	topManifestBlob, _, err := s.impl.GetManifest(ctx, rawSource, nil)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	mani, err := s.impl.OCI1FromManifest(topManifestBlob)
	if err != nil {
		return nil, fmt.Errorf("convert manifest: %w", err)
	}

	manifestBytes, err := s.impl.ToJSON(mani)
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	artifact := &Artifact{
		namedRef: unknownRef{},
		manifest: mani,
		digest:   digest.FromBytes(manifestBytes),
	}

	if val, ok := item.ManifestDescriptor.Annotations[specs.AnnotationRefName]; ok {
		namedRef, err := reference.ParseNormalizedNamed(val)
		if err != nil {
			log.Warnf(ctx, "Failed to parse annotation ref %s with the error %s", val, err)
		}

		artifact.namedRef = namedRef
	}

	return artifact, nil
}

func (s *Store) artifactData(ctx context.Context, nameOrDigest string, maxArtifactSize uint64) (res []ArtifactData, err error) {
	artifact, nameIsDigest, err := s.getByNameOrDigest(ctx, nameOrDigest)
	if err != nil {
		return nil, fmt.Errorf("get artifact by name or digest: %w", err)
	}

	if nameIsDigest {
		nameOrDigest = artifact.Reference()
	}

	imageReference, err := s.impl.LayoutNewReference(s.rootPath, nameOrDigest)
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

	layerInfos := s.impl.LayerInfos(artifact.manifest)
	for i := range layerInfos {
		layer := &layerInfos[i]
		title := artifactName(layer.Annotations)

		layerBytes, err := s.readBlob(ctx, imageSource, layer, maxArtifactSize)
		if err != nil {
			return nil, fmt.Errorf("read artifact blob: %w", err)
		}

		readSize += uint64(len(layerBytes))
		if readSize > maxArtifactSize {
			return nil, fmt.Errorf("exceeded maximum allowed artifact size of %d bytes", maxArtifactSize)
		}

		res = append(res, ArtifactData{
			title:  title,
			digest: layer.Digest,
			data:   layerBytes,
		})
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

func (s *Store) getByNameOrDigest(ctx context.Context, strRef string) (*Artifact, bool, error) {
	if strRef == "" {
		return nil, false, errors.New("empty name or digest")
	}

	artifacts, err := s.List(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("list artifacts: %w", err)
	}

	// if strRef is a just digest or short digest
	if idx := slices.IndexFunc(artifacts, func(a *Artifact) bool { return strings.HasPrefix(a.digest.Encoded(), strRef) }); len(strRef) >= 3 && idx != -1 {
		return artifacts[idx], true, nil
	}

	// if strRef is named reference
	candidates, err := s.impl.CandidatesForPotentiallyShortImageName(s.systemContext, strRef)
	if err != nil {
		return nil, false, fmt.Errorf("get candidates for potentially short image name: %w", err)
	}

	for _, candidate := range candidates {
		for _, artifact := range artifacts {
			if candidate.String() == artifact.Reference() || candidate.String() == artifact.CanonicalName() {
				return artifact, false, nil
			}
		}
	}

	return nil, false, fmt.Errorf("%w with name or digest of: %s", ErrNotFound, strRef)
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

// BlobMountPath represents a mapping of a source path in the blob directory to a file name in the artifact.
type BlobMountPath struct {
	// Source path of the blob, i.e. full path in the blob dir.
	SourcePath string
	// Name of the file in the artifact.
	Name string
}

// BlobMountPaths retrieves the local file paths for all blobs in the provided artifact and returns them as BlobMountPath slices.
func (s *Store) BlobMountPaths(ctx context.Context, artifact *Artifact, sys *types.SystemContext) ([]BlobMountPath, error) {
	ref, err := layout.NewReference(s.rootPath, artifact.Reference())
	if err != nil {
		return nil, fmt.Errorf("failed to get an image reference: %w", err)
	}

	src, err := ref.NewImageSource(ctx, sys)
	if err != nil {
		return nil, fmt.Errorf("failed to get an image source: %w", err)
	}

	defer src.Close()

	mountPaths := make([]BlobMountPath, 0, len(artifact.Manifest().Layers))

	for _, l := range artifact.Manifest().Layers {
		path, err := layout.GetLocalBlobPath(ctx, src, l.Digest)
		if err != nil {
			return nil, fmt.Errorf("failed to get a local blob path: %w", err)
		}

		name := artifactName(l.Annotations)
		if name == "" {
			log.Warnf(ctx, "Unable to find name for artifact layer which makes it not mountable")

			continue
		}

		mountPaths = append(mountPaths, BlobMountPath{
			SourcePath: path,
			Name:       name,
		})
	}

	return mountPaths, nil
}

func artifactName(annotations map[string]string) string {
	if name, ok := annotations[specs.AnnotationTitle]; ok {
		return name
	}

	if name, ok := annotations[modelSpec.AnnotationFilepath]; ok {
		return name
	}

	return ""
}
