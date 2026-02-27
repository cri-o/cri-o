package ociartifact

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	modelSpec "github.com/modelpack/model-spec/specs-go/v1"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/common/libimage"
	libart "go.podman.io/common/pkg/libartifact"
	libartTypes "go.podman.io/common/pkg/libartifact/types"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/types"

	"github.com/cri-o/cri-o/internal/log"
)

// ErrNotFound is indicating that the artifact could not be found in the storage.
var ErrNotFound = libartTypes.ErrArtifactNotExist

// ErrIsAnImage is returned when the reference is a container image, not an OCI artifact.
var ErrIsAnImage = errors.New("reference is a container image, not an OCI artifact")

// imageMimeTypes lists manifest MIME types that correspond to standard
// container image formats (OCI and Docker v2 schema variants).
var imageMimeTypes = []string{
	specs.MediaTypeImageManifest,
	manifest.DockerV2Schema2MediaType,
	manifest.DockerV2Schema1MediaType,
	manifest.DockerV2Schema1SignedMediaType,
}

// configMediaTypes lists config MIME types that indicate a standard
// container image configuration. An empty string is included because
// some images omit the config media type entirely.
var configMediaTypes = []string{
	"", // empty, treated as a container image config by convention
	specs.MediaTypeImageConfig,
	manifest.DockerV2Schema2ConfigMediaType,
}

// Store is the main structure to build an artifact storage.
type Store struct {
	libartifactStore LibartifactStore
	impl             Impl

	// rootPath is required for BlobMountPaths.
	rootPath string
}

// NewStore creates a new OCI artifact store.
func NewStore(rootPath string, systemContext *types.SystemContext) (*Store, error) {
	storePath := filepath.Join(rootPath, "artifacts")

	store, err := libart.NewArtifactStore(storePath, systemContext)
	if err != nil {
		return nil, err
	}

	return &Store{
		libartifactStore: &artifactStore{store},
		rootPath:         storePath,
		impl:             &defaultImpl{},
	}, nil
}

// Pull tries to pull the artifact and returns the manifest bytes if the
// provided reference is a valid OCI artifact.
//
// copyOptions will be passed down to libimage.
func (s *Store) Pull(
	ctx context.Context,
	ref types.ImageReference,
	opts *libimage.CopyOptions,
) (manifestDigest *digest.Digest, err error) {
	// Reject regular container images early. If a container image was
	// pulled into the artifact store, it would not be usable as an image.
	if err := s.EnsureNotContainerImage(ctx, ref); err != nil {
		return nil, fmt.Errorf("image reference: %w", err)
	}

	strRef := ref.DockerReference().String()

	log.Infof(ctx, "Pulling OCI artifact %s", strRef)

	artRef, err := libart.NewArtifactReference(strRef)
	if err != nil {
		return nil, fmt.Errorf("invalid reference: %w", err)
	}

	dgst, err := s.libartifactStore.Pull(ctx, artRef, *opts)
	if err != nil {
		return nil, fmt.Errorf("pull artifact: %w", err)
	}

	return &dgst, nil
}

// EnsureNotContainerImage inspects the manifest at ref and returns
// ErrIsAnImage when the reference points to a regular container image
// rather than an OCI artifact. Multi-architecture manifest lists are
// resolved to the current platform before inspection.
func (s *Store) EnsureNotContainerImage(ctx context.Context, ref types.ImageReference) error {
	// Fetch the top-level manifest (or manifest list) for the given reference.
	// Passing nil as the instance digest retrieves the root manifest.
	manifestBytes, mimeType, err := s.impl.GetManifestFromRef(ctx, ref, s.libartifactStore.SystemContext(), nil)
	if err != nil {
		return fmt.Errorf("get manifest from ref: %w", err)
	}

	// If it's a manifest list/index, resolve the correct instance for the
	// current platform. This is necessary because multi-arch images contain
	// multiple platform-specific manifests, and we need the one matching
	// the host OS and architecture to inspect its config.
	if manifest.MIMETypeIsMultiImage(mimeType) {
		list, err := manifest.ListFromBlob(manifestBytes, mimeType)
		if err != nil {
			return fmt.Errorf("parse manifest list: %w", err)
		}

		instanceDigest, err := s.impl.ChooseInstance(list, s.libartifactStore.SystemContext())
		if err != nil {
			return fmt.Errorf("choose manifest instance: %w", err)
		}

		manifestBytes, mimeType, err = s.impl.GetManifestFromRef(ctx, ref, s.libartifactStore.SystemContext(), &instanceDigest)
		if err != nil {
			return fmt.Errorf("get instance manifest: %w", err)
		}
	}

	// Extract the config descriptor's media type, which indicates what kind
	// of content this manifest describes (e.g., a container image config vs.
	// an arbitrary artifact config).
	m, err := manifest.FromBlob(manifestBytes, mimeType)
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	mediaType := m.ConfigInfo().MediaType

	// If both the manifest and config media types match known container image
	// types, perform additional checks to see if the manifest declares an
	// OCI artifact type. If it does, this is an artifact rather than an image.
	if slices.Contains(imageMimeTypes, mimeType) && slices.Contains(configMediaTypes, mediaType) {
		ociManifest, err := manifest.OCI1FromManifest(manifestBytes)
		// Non-OCI manifests (e.g. Docker v2 schema 2) are expected to fail
		// parsing here, which means they are regular container images.
		if err != nil {
			log.Debugf(ctx, "Failed to parse OCI 1 manifest: %v", err)

			return ErrIsAnImage
		}

		// No artifact type set, assume an image. Per the OCI image spec,
		// the artifactType field distinguishes artifacts from regular images.
		if ociManifest.ArtifactType == "" {
			return ErrIsAnImage
		}

		log.Debugf(ctx, "Found artifact type: %s", ociManifest.ArtifactType)
	}

	return nil
}

// List creates a slice of all available artifacts.
func (s *Store) List(ctx context.Context) (res []*Artifact, err error) {
	arts, err := s.libartifactStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}

	for _, art := range arts {
		res = append(res, NewArtifact(art))
	}

	return res, nil
}

// Status retrieves the artifact by referencing a name or digest.
// Returns ErrNotFound if the artifact is not available.
func (s *Store) Status(ctx context.Context, nameOrDigest string) (*Artifact, error) {
	artRef, err := libart.NewArtifactStorageReference(nameOrDigest)
	if err != nil {
		return nil, fmt.Errorf("invalid nameOrDigest: %w", err)
	}

	artifact, err := s.libartifactStore.Inspect(ctx, artRef)
	if err != nil {
		return nil, fmt.Errorf("inspect artifact: %w", err)
	}

	return NewArtifact(artifact), nil
}

// Remove deletes a name or digest from the artifact store.
// Returns ErrNotFound if the artifact is not available.
func (s *Store) Remove(ctx context.Context, nameOrDigest string) error {
	artRef, err := libart.NewArtifactStorageReference(nameOrDigest)
	if err != nil {
		return fmt.Errorf("invalid nameOrDigest: %w", err)
	}

	_, err = s.libartifactStore.Remove(ctx, artRef)

	return err
}

// BlobMountPaths retrieves the local file paths for all blobs in the provided artifact and returns them as BlobMountPath slices.
// This should be replaced by BlobMountPaths in c/common, but it doesn't support modelpack, so we keep it here for now.
func (s *Store) BlobMountPaths(ctx context.Context, artifact *Artifact, sys *types.SystemContext) ([]libartTypes.BlobMountPath, error) {
	ref, err := layout.NewReference(s.rootPath, artifact.Reference())
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

// RootPath returns the root path of the store.
func (s *Store) RootPath() string {
	return s.rootPath
}
