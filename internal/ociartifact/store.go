package ociartifact

import (
	"context"
	"fmt"
	"path/filepath"

	modelSpec "github.com/modelpack/model-spec/specs-go/v1"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/common/libimage"
	libart "go.podman.io/common/pkg/libartifact"
	libartTypes "go.podman.io/common/pkg/libartifact/types"
	"go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/types"

	"github.com/cri-o/cri-o/internal/log"
)

// ErrNotFound is indicating that the artifact could not be found in the storage.
var ErrNotFound = libartTypes.ErrArtifactNotExist

// Store is the main structure to build an artifact storage.
type Store struct {
	libartifactStore LibartifactStore

	// It's required for BlobMountPaths.
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
		libartifactStore: store,
		rootPath:         storePath,
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
	strRef := ref.DockerReference().String()

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
