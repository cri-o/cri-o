package datastore

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/common/libimage"
	"go.podman.io/common/pkg/libartifact"
	libartTypes "go.podman.io/common/pkg/libartifact/types"
	"go.podman.io/image/v5/image"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/pkg/blobinfocache/none"
	"go.podman.io/image/v5/types"

	"github.com/cri-o/cri-o/internal/log"
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

// Store handles pulling artifact data and reading blobs using the podman
// libartifact store directly.
type Store struct {
	store         LibartifactStore
	impl          Impl
	storePath     string
	systemContext *types.SystemContext
}

// New creates a new OCI artifact data store.
func New(rootPath string, systemContext *types.SystemContext) (*Store, error) {
	storePath := filepath.Join(rootPath, "artifacts")

	artStore, err := libartifact.NewArtifactStore(storePath, systemContext)
	if err != nil {
		return nil, fmt.Errorf("create artifact store: %w", err)
	}

	return &Store{
		store:         artStore,
		impl:          &defaultImpl{},
		storePath:     storePath,
		systemContext: systemContext,
	}, nil
}

// PullOptions can be used to customize the pull behavior.
type PullOptions struct {
	// MaxSize is the maximum size of the artifact to be allowed to stay
	// in-memory. This is only useful when requesting the artifact data using
	// PullData.
	// Will be set to a default of 1MiB if not specified (zero) or below zero.
	MaxSize uint64

	// CopyOptions are the copy options passed down to libimage.
	CopyOptions *libimage.CopyOptions
}

// PullData downloads the artifact into the local storage and returns its data.
func (s *Store) PullData(
	ctx context.Context,
	ref string,
	opts *PullOptions,
) ([]ArtifactData, error) {
	opts = sanitizeOptions(opts)

	log.Infof(ctx, "Pulling OCI artifact from ref: %s", ref)

	artRef, err := s.impl.NewArtifactReference(ref)
	if err != nil {
		return nil, fmt.Errorf("create artifact reference: %w", err)
	}

	if _, err := s.store.Pull(ctx, artRef, *opts.CopyOptions); err != nil {
		return nil, fmt.Errorf("pull artifact: %w", err)
	}

	blobPaths, err := s.store.BlobMountPaths(
		ctx,
		artRef.ToArtifactStoreReference(),
		&libartTypes.BlobMountPathOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("get blob mount paths: %w", err)
	}

	return s.readBlobData(blobPaths, opts.MaxSize)
}

func (s *Store) readBlobData(
	blobPaths []libartTypes.BlobMountPath,
	maxSize uint64,
) ([]ArtifactData, error) {
	var res []ArtifactData

	totalSize := uint64(0)

	for _, bp := range blobPaths {
		remaining := int64(maxSize - totalSize)

		data, err := s.impl.ReadFile(bp.SourcePath, remaining+1)
		if err != nil {
			return nil, fmt.Errorf("read blob file: %w", err)
		}

		totalSize += uint64(len(data))
		if totalSize > maxSize {
			return nil, fmt.Errorf("exceeded maximum allowed artifact size of %d bytes", maxSize)
		}

		res = append(res, ArtifactData{data: data})
	}

	return res, nil
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

// PullManifestOnly fetches only the manifest and config blob from the remote
// registry and records them in the local OCI layout store without downloading
// any layer blobs.  It is intended for use when layer data is retrieved
// externally (e.g. inside a confidential VM by the kata-agent).
// Returns the digest of the stored manifest.
func (s *Store) PullManifestOnly(
	ctx context.Context,
	ref types.ImageReference,
	opts *libimage.CopyOptions,
) (*digest.Digest, error) {
	strRef := ref.DockerReference().String()

	log.Infof(ctx, "Pulling OCI manifest and config (no layers): %s", strRef)

	// Merge per-request auth credentials from opts into a local copy of the
	// system context, mirroring what libimage.NewCopier does.
	sys := s.systemContext
	if sys == nil {
		sys = &types.SystemContext{}
	}

	if opts != nil && (opts.Username != "" || opts.AuthFilePath != "") {
		sysCopy := *sys

		if opts.Username != "" {
			sysCopy.DockerAuthConfig = &types.DockerAuthConfig{
				Username: opts.Username,
				Password: opts.Password,
			}
		}

		if opts.AuthFilePath != "" {
			sysCopy.AuthFilePath = opts.AuthFilePath
		}

		sys = &sysCopy
	}

	// Open the remote source once to fetch both the manifest and config blob.
	remoteSrc, err := ref.NewImageSource(ctx, sys)
	if err != nil {
		return nil, fmt.Errorf("open remote source: %w", err)
	}
	defer remoteSrc.Close()

	manifestBytes, mimeType, err := remoteSrc.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	// If the registry returned a manifest list, resolve to the
	// platform-specific instance for the current host.
	if manifest.MIMETypeIsMultiImage(mimeType) {
		list, err := manifest.ListFromBlob(manifestBytes, mimeType)
		if err != nil {
			return nil, fmt.Errorf("parse manifest list: %w", err)
		}

		instanceDigest, err := list.ChooseInstance(sys)
		if err != nil {
			return nil, fmt.Errorf("choose manifest instance: %w", err)
		}

		manifestBytes, mimeType, err = remoteSrc.GetManifest(ctx, &instanceDigest)
		if err != nil {
			return nil, fmt.Errorf("get platform manifest: %w", err)
		}
	}

	// Parse the (platform-specific) manifest to obtain the config descriptor.
	parsedManifest, err := manifest.FromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	configInfo := parsedManifest.ConfigInfo()

	// Open the local OCI layout destination inside the libartifact store path.
	// Writing here makes the entry visible to libartifact's List/Remove.
	ir, err := layout.NewReference(s.storePath, strRef)
	if err != nil {
		return nil, fmt.Errorf("layout reference: %w", err)
	}

	imgDest, err := ir.NewImageDestination(ctx, sys)
	if err != nil {
		return nil, fmt.Errorf("image destination: %w", err)
	}
	defer imgDest.Close()

	// Fetch the config blob from remote and write it locally so that
	// PullConfig can read the OCI image config after this call.
	configRC, _, err := remoteSrc.GetBlob(
		ctx,
		types.BlobInfo{Digest: configInfo.Digest, Size: configInfo.Size},
		none.NoCache,
	)
	if err != nil {
		return nil, fmt.Errorf("get config blob: %w", err)
	}
	defer configRC.Close()

	if _, err := imgDest.PutBlob(
		ctx,
		configRC,
		types.BlobInfo{Digest: configInfo.Digest, Size: configInfo.Size},
		none.NoCache,
		true,
	); err != nil {
		return nil, fmt.Errorf("store config blob: %w", err)
	}

	if err := imgDest.PutManifest(ctx, manifestBytes, nil); err != nil {
		return nil, fmt.Errorf("put manifest: %w", err)
	}

	unparsed := &manifestOnlyUnparsed{ir: ir, manifestBytes: manifestBytes, mimeType: mimeType}
	if err := imgDest.Commit(ctx, unparsed); err != nil {
		return nil, fmt.Errorf("commit manifest: %w", err)
	}

	dgst := digest.FromBytes(manifestBytes)

	return &dgst, nil
}

// ImageSource returns an ImageSource for the entry identified by dgstStr.
// dgstStr is the hex-encoded manifest digest (without the "sha256:" prefix).
// The caller is responsible for closing the returned ImageSource.
//
// It searches the OCI layout index directly so that it works regardless of
// whether the manifest is OCI v1 or Docker v2 format.
func (s *Store) ImageSource(ctx context.Context, dgstStr string) (types.ImageSource, error) {
	entries, err := layout.List(s.storePath)
	if err != nil {
		return nil, fmt.Errorf("list OCI layout: %w", err)
	}

	sys := s.systemContext
	if sys == nil {
		sys = &types.SystemContext{}
	}

	for i := range entries {
		if strings.HasPrefix(entries[i].ManifestDescriptor.Digest.Encoded(), dgstStr) {
			return entries[i].Reference.NewImageSource(ctx, sys)
		}
	}

	return nil, fmt.Errorf("no artifact found with digest %q", dgstStr)
}

// List returns all entries currently recorded in the libartifact OCI layout store.
func (s *Store) List(ctx context.Context) ([]*Artifact, error) {
	arts, err := s.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}

	result := make([]*Artifact, 0, len(arts))
	for _, art := range arts {
		result = append(result, newArtifact(art))
	}

	return result, nil
}

// Remove deletes the entry identified by nameOrDigest from the store.
func (s *Store) Remove(ctx context.Context, nameOrDigest string) error {
	artRef, err := libartifact.NewArtifactStorageReference(nameOrDigest)
	if err != nil {
		return fmt.Errorf("invalid nameOrDigest: %w", err)
	}

	if _, err := s.store.Remove(ctx, artRef); err != nil {
		return fmt.Errorf("remove artifact: %w", err)
	}

	return nil
}

// manifestOnlyUnparsed is a minimal types.UnparsedImage used when committing
// a manifest-only entry to the OCI layout store (no layer blobs written).
type manifestOnlyUnparsed struct {
	ir            types.ImageReference
	manifestBytes []byte
	mimeType      string
}

func (m *manifestOnlyUnparsed) Reference() types.ImageReference { return m.ir }

func (m *manifestOnlyUnparsed) Manifest(
	_ context.Context,
) (manifestBlob []byte, mimeType string, err error) {
	return m.manifestBytes, m.mimeType, nil
}

func (m *manifestOnlyUnparsed) Signatures(_ context.Context) ([][]byte, error) {
	return [][]byte{}, nil
}

// PullConfig reads the OCI image config for an entry previously stored by
// PullManifestOnly.  refStr is the full image reference string used during the
// original pull (e.g. "docker.io/library/ubuntu:latest").
func (s *Store) PullConfig(ctx context.Context, refStr string) (*specs.Image, error) {
	imageReference, err := layout.NewReference(s.storePath, refStr)
	if err != nil {
		return nil, fmt.Errorf("create layout reference: %w", err)
	}

	sys := s.systemContext
	if sys == nil {
		sys = &types.SystemContext{}
	}

	imageSource, err := imageReference.NewImageSource(ctx, sys)
	if err != nil {
		return nil, fmt.Errorf("build image source: %w", err)
	}

	defer func() {
		if err := imageSource.Close(); err != nil {
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
		manifestList, err := manifest.ListFromBlob(topManifest, topMIMEType)
		if err != nil {
			return nil, fmt.Errorf("parse manifest list: %w", err)
		}

		instanceDigest, err := manifestList.ChooseInstance(sys)
		if err != nil {
			return nil, fmt.Errorf("choose manifest instance: %w", err)
		}

		unparsedInstance = image.UnparsedInstance(imageSource, &instanceDigest)
	}

	sourcedImage, err := image.FromUnparsedImage(ctx, sys, unparsedInstance)
	if err != nil {
		return nil, fmt.Errorf("build image from unparsed: %w", err)
	}

	return sourcedImage.OCIConfig(ctx)
}
