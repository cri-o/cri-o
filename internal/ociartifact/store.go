package ociartifact

import (
	"context"
	"fmt"
	"path/filepath"

	modelSpec "github.com/modelpack/model-spec/specs-go/v1"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
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
	rootPath         string
	additionalPaths  []string
	additionalStores []LibartifactStore
}

// NewStore creates a new OCI artifact store.
func NewStore(rootPath string, additionalPaths []string, systemContext *types.SystemContext) (*Store, error) {
	storePath := filepath.Join(rootPath, "artifacts")

	store, err := libart.NewArtifactStore(storePath, systemContext)
	if err != nil {
		return nil, err
	}

	// Configure additional stores (RO)
	var additionalStores []LibartifactStore
	var validAdditionalPaths []string

	for _, path := range additionalPaths {
		addPath := filepath.Join(path, "artifacts")

		addStore, err := libart.NewArtifactStore(addPath, systemContext)
		if err != nil {
			logrus.WithFields(map[string]interface{}{
				"path": addPath,
				"err":  err,
			}).Warn("skipping additional artifact store")
			continue
		}
		additionalStores = append(additionalStores, addStore)
		validAdditionalPaths = append(validAdditionalPaths, addPath)
	}

	return &Store{
		libartifactStore: store,
		rootPath:         storePath,
		additionalPaths:  validAdditionalPaths,
		additionalStores: additionalStores,
	}, nil
}

// Pull tries to pull the artifact and returns the manifest bytes if the
// provided reference is a valid OCI artifact.
func (s *Store) Pull(
	ctx context.Context,
	ref types.ImageReference,
	opts *libimage.CopyOptions,
) (manifestDigest *digest.Digest, err error) {
	strRef := ref.DockerReference().String()

	// Check if it already exists in any store (including RO) to avoid pulling
	if art, err := s.Status(ctx, strRef); err == nil {
		log.Infof(ctx, "Artifact %s found locally, skipping pull", strRef)
		dgst := art.Digest()
		return &dgst, nil
	}

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
	var arts []*Artifact

	// Get from additional stores first (Prioritized)
	for i, addStore := range s.additionalStores {
		addArts, err := addStore.List(ctx)
		if err != nil {
			continue
		}
		for _, art := range addArts {
			arts = append(arts, NewArtifact(art, s.additionalPaths[i]))
		}
	}

	// Get from main store
	mainArts, err := s.libartifactStore.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list artifacts from main store: %w", err)
	}
	for _, art := range mainArts {
		arts = append(arts, NewArtifact(art, s.rootPath))
	}

	// Deduplicate preserving priority
	seen := make(map[string]struct{})
	for _, artifact := range arts {
		encoded := artifact.Digest().Encoded()
		if _, ok := seen[encoded]; ok {
			continue
		}
		seen[encoded] = struct{}{}
		res = append(res, artifact)
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

	// Check additional stores first (Prioritized)
	for i, store := range s.additionalStores {
		if artifact, err := store.Inspect(ctx, artRef); err == nil {
			return NewArtifact(artifact, s.additionalPaths[i]), nil
		}
	}

	// Check main store
	artifact, err := s.libartifactStore.Inspect(ctx, artRef)
	if err == nil {
		return NewArtifact(artifact, s.rootPath), nil
	}

	return nil, fmt.Errorf("inspect artifact: %w", err)
}

// Remove deletes a name or digest from the artifact store.
// Returns ErrNotFound if the artifact is not available.
func (s *Store) Remove(ctx context.Context, nameOrDigest string) error {
	artRef, err := libart.NewArtifactStorageReference(nameOrDigest)
	if err != nil {
		return fmt.Errorf("invalid nameOrDigest: %w", err)
	}

	// Only remove from the main writeable store
	_, err = s.libartifactStore.Remove(ctx, artRef)

	return err
}

// BlobMountPaths retrieves the local file paths for all blobs in the provided artifact and returns them as BlobMountPath slices.
func (s *Store) BlobMountPaths(ctx context.Context, artifact *Artifact, sys *types.SystemContext) ([]libartTypes.BlobMountPath, error) {
	// The rootPath is now inherently known by the artifact itself
	rootPath := artifact.RootPath()

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

// RootPath returns the root path of the store.
func (s *Store) RootPath() string {
	return s.rootPath
}
