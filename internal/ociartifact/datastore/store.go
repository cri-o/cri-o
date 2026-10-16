package datastore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/common/libimage"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/image"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/oci/layout"
	"go.podman.io/image/v5/pkg/blobinfocache"
	"go.podman.io/image/v5/pkg/blobinfocache/none"
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

	storePath     string
	systemContext *types.SystemContext
	impl          Impl
}

// New creates a new OCI artifact data store.
func New(rootPath string, systemContext *types.SystemContext) (*Store, error) {
	// The datastore only handles artifacts pulled into the main store.
	// Additional read-only stores are not threaded through here (we pass
	// nil for additionalPaths) since the datastore is used for in-memory
	// artifact data managed by the main CRI-O lifecycle.
	ociStore, err := ociartifact.NewStore(rootPath, nil, systemContext, nil)
	if err != nil {
		return nil, fmt.Errorf("create OCI artifact store: %w", err)
	}

	return &Store{
		Store:         ociStore,
		storePath:     ociStore.RootPath(),
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
	configRC, _, err := remoteSrc.GetBlob(ctx, types.BlobInfo{Digest: configInfo.Digest, Size: configInfo.Size}, none.NoCache)
	if err != nil {
		return nil, fmt.Errorf("get config blob: %w", err)
	}
	defer configRC.Close()

	if _, err := imgDest.PutBlob(ctx, configRC, types.BlobInfo{Digest: configInfo.Digest, Size: configInfo.Size}, none.NoCache, true); err != nil {
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

// manifestOnlyUnparsed is a minimal types.UnparsedImage used when committing
// a manifest-only entry to the OCI layout store (no layer blobs written).
type manifestOnlyUnparsed struct {
	ir            types.ImageReference
	manifestBytes []byte
	mimeType      string
}

func (m *manifestOnlyUnparsed) Reference() types.ImageReference { return m.ir }

func (m *manifestOnlyUnparsed) Manifest(_ context.Context) (manifestBlob []byte, mimeType string, err error) {
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
