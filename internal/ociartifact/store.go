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

	modelSpec "github.com/modelpack/model-spec/specs-go/v1"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	libartStore "go.podman.io/common/pkg/libartifact/store"
	libartTypes "go.podman.io/common/pkg/libartifact/types"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/pkg/blobinfocache"
	"go.podman.io/image/v5/types"

	"github.com/cri-o/cri-o/internal/log"
)

// defaultMaxArtifactSize is the default size per artifact data.
const defaultMaxArtifactSize = 1 * 1024 * 1024 // 1 MiB

// ErrNotFound is indicating that the artifact could not be found in the storage.
var ErrNotFound = errors.New("no artifact found")

// Store is the main structure to build an artifact storage.
type Store struct {
	LibartifactStore

	rootPath         string
	additionalPaths  []string
	additionalStores []LibartifactStore
	impl             Impl
}

// NewStore creates a new OCI artifact store.
func NewStore(rootPath string, additionalPaths []string, systemContext *types.SystemContext) (*Store, error) {
	storePath := filepath.Join(rootPath, "artifacts")
	store, err := libartStore.NewArtifactStore(storePath, systemContext)
	if err != nil {
		return nil, err
	}

	// Configure additional stores (RO)
	var additionalStores []LibartifactStore
	var validAdditionalPaths []string

	for _, path := range additionalPaths {
		// Assume the pattern is to have an "artifacts" directory within the configured path,
		// similar to how additionalimagestores works in containers/storage.
		addPath := filepath.Join(path, "artifacts")

		addStore, err := libartStore.NewArtifactStore(addPath, systemContext)
		if err != nil {
			// Just ignore invalid stores to prevent breaking CRI-O startup
			continue
		}
		additionalStores = append(additionalStores, RealLibartifactStore{addStore})
		validAdditionalPaths = append(validAdditionalPaths, addPath)
	}

	return &Store{
		LibartifactStore: RealLibartifactStore{store},
		rootPath:         storePath,
		additionalPaths:  validAdditionalPaths,
		additionalStores: additionalStores,
		impl:             &defaultImpl{},
	}, nil
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
	log.Infof(ctx, "Checking/Pulling OCI artifact from ref: %s", ref)

	var manifestDigestEncoded string

	existingArt, _, err := s.getByNameOrDigest(ctx, ref)

	if err == nil && existingArt != nil {
		log.Infof(ctx, "Artifact %s found locally, skipping pull", ref)
		manifestDigestEncoded = existingArt.digest.Encoded()
	} else {
		dockerRef, err := s.getImageReference(ref)
		if err != nil {
			return nil, fmt.Errorf("failed to get image reference: %w", err)
		}

		manifestDigest, err := s.PullManifest(ctx, dockerRef, opts.CopyOptions)
		if err != nil {
			return nil, fmt.Errorf("pull artifact: %w", err)
		}
		manifestDigestEncoded = manifestDigest.Encoded()
	}

	artifactData, err := s.artifactData(ctx, manifestDigestEncoded, opts.MaxSize)
	if err != nil {
		return nil, fmt.Errorf("get artifact data: %w", err)
	}

	return artifactData, nil
}

// PullManifest tries to pull the artifact and returns the manifest bytes if the
// provided reference is a valid OCI artifact.
//
// copyOptions will be passed down to libimage.
func (s *Store) PullManifest(
	ctx context.Context,
	ref types.ImageReference,
	opts *libimage.CopyOptions,
) (manifestDigest *digest.Digest, err error) {
	strRef := s.impl.DockerReferenceString(ref)

	dgst, err := s.Pull(ctx, strRef, *opts)
	if err != nil {
		return nil, fmt.Errorf("pull artifact: %w", err)
	}

	return &dgst, nil
}

// List creates a slice of all available artifacts.
func (s *Store) List(ctx context.Context) (res []*Artifact, err error) {
	arts, err := s.LibartifactStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list artifacts from main store: %w", err)
	}

	for _, addStore := range s.additionalStores {
		addArts, err := addStore.List(ctx)
		if err != nil {
			continue
		}
		arts = append(arts, addArts...)
	}

	for _, art := range arts {
		artifact, err := s.buildArtifact(ctx, art)
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
	artifact, _, err := s.getByNameOrDigest(ctx, nameOrDigest)
	if err != nil {
		return fmt.Errorf("get artifact by name or digest: %w", err)
	}

	_, err = s.LibartifactStore.Remove(ctx, artifact.Reference())

	return err
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

func (s *Store) buildArtifact(ctx context.Context, art *libartifact.Artifact) (*Artifact, error) {
	dgst, err := art.GetDigest()
	if err != nil {
		return nil, fmt.Errorf("get digest: %w", err)
	}

	artifact := &Artifact{
		Artifact: art,
		namedRef: unknownRef{},
		digest:   *dgst,
	}

	if art.Name != "" {
		namedRef, err := reference.ParseNormalizedNamed(art.Name)
		if err != nil {
			log.Warnf(ctx, "Failed to parse artifact name %s with the error %s", art.Name, err)

			namedRef = unknownRef{}
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

	rootPath, err := s.resolveRootPath(ctx, artifact.digest.Encoded())
	if err != nil {
		rootPath = s.rootPath
	}

	imageReference, err := s.impl.LayoutNewReference(rootPath, nameOrDigest)
	if err != nil {
		return nil, fmt.Errorf("create new reference: %w", err)
	}

	imageSource, err := s.impl.NewImageSource(ctx, imageReference, s.SystemContext())
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

		res = append(res, ArtifactData{
			data: layerBytes,
		})
	}

	return res, nil
}

func (s *Store) readBlob(ctx context.Context, src types.ImageSource, layer *manifest.LayerInfo, maxArtifactSize uint64) ([]byte, error) {
	bic := blobinfocache.DefaultCache(s.SystemContext())

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
// Returns ErrNotFound if no matching artifact exists.
// TODO: replace with GetByNameOrDigest in libartifact.
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
	candidates, err := s.impl.CandidatesForPotentiallyShortImageName(s.SystemContext(), strRef)
	if err != nil {
		// If there are no artifacts in the store, return ErrNotFound regardless of name validation errors
		// This maintains backward compatibility where invalid names simply weren't found
		if len(artifacts) == 0 {
			return nil, false, fmt.Errorf("%w with name or digest of: %s", ErrNotFound, strRef)
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

// BlobMountPaths retrieves the local file paths for all blobs in the provided artifact and returns them as BlobMountPath slices.
// This should be replaced by BlobMountPaths in c/common, but it doesn't support modelpack, so we keep it here for now.
func (s *Store) BlobMountPaths(ctx context.Context, artifact *Artifact, sys *types.SystemContext) ([]libartTypes.BlobMountPath, error) {
	rootPath, err := s.resolveRootPath(ctx, artifact.digest.Encoded())
	if err != nil {
		return nil, fmt.Errorf("failed to resolve artifact path: %w", err)
	}

	ref, err := layout.NewReference(rootPath, artifact.Reference())
	if err != nil {
		return nil, fmt.Errorf("failed to get an image reference: %w", err)
	}

	src, err := ref.NewImageSource(ctx, sys)
	if err != nil {
		return nil, fmt.Errorf("failed to get an image source: %w", err)
	}

	defer src.Close()

	mountPaths := make([]libartTypes.BlobMountPath, 0, len(artifact.Manifest.Layers))

	for _, l := range artifact.Manifest.Layers {
		path, err := layout.GetLocalBlobPath(ctx, src, l.Digest)
		if err != nil {
			return nil, fmt.Errorf("failed to get a local blob path: %w", err)
		}

		name := artifactName(l.Annotations)
		if name == "" {
			log.Warnf(ctx, "Unable to find name for artifact layer which makes it not mountable")

			continue
		}

		mountPaths = append(mountPaths, libartTypes.BlobMountPath{
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

// resolveRootPath finds which directory (main or additional) the artifact resides in.
func (s *Store) resolveRootPath(ctx context.Context, nameOrDigest string) (string, error) {
	arts, _ := s.LibartifactStore.List(ctx)
	if s.containsArtifact(arts, nameOrDigest) {
		return s.rootPath, nil
	}

	for i, store := range s.additionalStores {
		arts, _ := store.List(ctx)
		if s.containsArtifact(arts, nameOrDigest) {
			return s.additionalPaths[i], nil
		}
	}

	return "", fmt.Errorf("artifact %s not found in any store", nameOrDigest)
}

// Helper to check if the artifact exists in the list (by digest or name)
func (s *Store) containsArtifact(artifacts []*libartifact.Artifact, ref string) bool {
	for _, art := range artifacts {
		dgst, _ := art.GetDigest()
		if strings.HasPrefix(dgst.Encoded(), ref) {
			return true
		}
		if art.Name == ref {
			return true
		}
	}
	return false
}

// SetImpl sets the implementation of the store.
// Used for testing only.
func (s *Store) SetImpl(impl Impl) {
	s.impl = impl
}

// SetFakeStore sets the LibartifactStore.
// Used for testing only.
func (s *Store) SetFakeStore(store LibartifactStore) {
	s.LibartifactStore = store
}

// FakeLibartifactStore is a wrapper for the LibartifactStore interface.
// It is exported to be used in tests.
type FakeLibartifactStore struct {
	LibartifactStore
}

// SetData sets the data of the artifact.
// Used for testing only.
func (a *ArtifactData) SetData(data []byte) {
	a.data = data
}
