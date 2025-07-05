package ociartifact

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	modelSpec "github.com/CloudNativeAI/model-spec/specs-go/v1"
	"github.com/containers/common/libimage"
	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/oci/layout"
	"github.com/containers/image/v5/pkg/blobinfocache"
	"github.com/containers/image/v5/types"
	"github.com/klauspost/compress/zstd"
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
	rootPath           string
	systemContext      *types.SystemContext
	impl               Impl
	extractArtifactDir string
}

// NewStore creates a new OCI artifact store.
func NewStore(rootPath string, systemContext *types.SystemContext) *Store {
	return &Store{
		rootPath:           filepath.Join(rootPath, "artifacts"),
		systemContext:      systemContext,
		impl:               &defaultImpl{},
		extractArtifactDir: filepath.Join(rootPath, "extracted-artifacts"),
	}
}

type unknownRef struct{}

func (unknownRef) String() string {
	return "unknown"
}

func (u unknownRef) Name() string {
	return u.String()
}

func (s *Store) getArtifactExtractDir(artifact *Artifact) (string, error) {
	// Parse manifest from artifact
	manifestBytes, err := s.impl.ToJSON(artifact.Manifest())
	if err != nil {
		return "", fmt.Errorf("marshal manifest: %w", err)
	}

	parsedManifest, err := s.impl.ManifestFromBlob(manifestBytes, specs.MediaTypeImageManifest)
	if err != nil {
		return "", fmt.Errorf("parse manifest: %w", err)
	}

	configInfo := parsedManifest.ConfigInfo()
	if configInfo.Digest == "" {
		return "", errors.New("artifact manifest has no config digest")
	}

	return filepath.Join(s.extractArtifactDir, configInfo.Digest.Encoded()), nil
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

	manifestBytes, mimeType, err := s.getManifestFromRef(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	log.Debugf(ctx, "Manifest mime type of %s: %s", strRef, mimeType)

	if mimeType == manifest.DockerV2ListMediaType {
		return nil, ErrIsAnImage
	}

	// At this point we are not sure if the reference is for an image or an artifact.
	// To verify whether the reference is an artifact or an image, it needs to parse
	// the manifest and see its media type.
	if mimeType == specs.MediaTypeImageIndex {
		manifestList, err := s.impl.ListFromBlob(manifestBytes, mimeType)
		if err != nil {
			return nil, fmt.Errorf("parse manifest from blob: %w", err)
		}

		instanceDigest, err := s.impl.ChooseInstance(manifestList, s.systemContext)
		if err != nil {
			return nil, fmt.Errorf("choose instance: %w", err)
		}

		ref, err = s.getImageReference(fmt.Sprintf("%s@%s", s.impl.DockerReferenceName(ref), instanceDigest))
		if err != nil {
			return nil, fmt.Errorf("failed to get image reference: %w", err)
		}

		manifestBytes, mimeType, err = s.getManifestFromRef(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("get manifest: %w", err)
		}
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

	configInfo := parsedManifest.ConfigInfo()
	if configInfo.Digest == "" {
		return nil, errors.New("manifest has no config digest")
	}

	extractDir := filepath.Join(s.extractArtifactDir, configInfo.Digest.Encoded())
	if err := s.extractArtifactLayers(ctx, strRef, parsedManifest, dst, extractDir); err != nil {
		return nil, fmt.Errorf("extract artifact layers: %w", err)
	}

	return manifestBytes, nil
}

// getManifestFromRef retrieves the manifest from the given image reference.
func (s *Store) getManifestFromRef(ctx context.Context, ref types.ImageReference) (manifestBytes []byte, mimeType string, err error) {
	src, err := s.impl.NewImageSource(ctx, ref, s.systemContext)
	if err != nil {
		return nil, "", fmt.Errorf("build image source: %w", err)
	}

	defer func() {
		if err := s.impl.CloseImageSource(src); err != nil {
			log.Warnf(ctx, "Unable to close image source: %v", err)
		}
	}()

	return s.impl.GetManifest(ctx, src, nil)
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
	Name        string
	cleanupFunc func()
	cleanupOnce *sync.Once
}

// Cleanup triggers the deletion of the extracted directory.
func (b *BlobMountPath) Cleanup() {
	if b.cleanupFunc != nil && b.cleanupOnce != nil {
		b.cleanupOnce.Do(b.cleanupFunc)
	}
}

// BlobMountPaths retrieves the local file paths for all blobs in the provided artifact and returns them as BlobMountPath slices.
func (s *Store) BlobMountPaths(ctx context.Context, artifact *Artifact, sys *types.SystemContext) ([]BlobMountPath, error) {
	ref, err := layout.NewReference(s.rootPath, artifact.Reference())
	if err != nil {
		return nil, fmt.Errorf("failed to get an image reference: %w", err)
	}

	extractDir, err := s.getArtifactExtractDir(artifact)
	if err != nil {
		return nil, fmt.Errorf("get artifact extract dir: %w", err)
	}

	cleanupFunc := func() {
		if err := os.RemoveAll(extractDir); err != nil {
			log.Warnf(ctx, "Failed to remove artifact extract directory %s: %v", extractDir, err)
		} else {
			log.Debugf(ctx, "Successfully removed artifact extract directory: %s", extractDir)
		}
	}
	cleanupOnce := &sync.Once{}

	// Cleanup on function exit if we encounter an error.
	cleanup := true
	defer func() {
		if cleanup {
			if err := os.RemoveAll(extractDir); err != nil {
				log.Warnf(ctx, "Failed to remove artifact extract directory on error %s: %v", extractDir, err)
			} else {
				log.Debugf(ctx, "Successfully removed artifact extract directory on error: %s", extractDir)
			}
		}
	}()

	src, err := ref.NewImageSource(ctx, sys)
	if err != nil {
		return nil, fmt.Errorf("failed to get an image source: %w", err)
	}
	defer src.Close()

	mountPaths := make([]BlobMountPath, 0, len(artifact.Manifest().Layers))
	data, err := os.ReadFile(filepath.Join(s.extractArtifactDir, "layer-map.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read layer map: %w", err)
	}

	layerMap := make(map[string]string)
	if err := json.Unmarshal(data, &layerMap); err != nil {
		return nil, fmt.Errorf("failed to parse layer map: %w", err)
	}

	for _, l := range artifact.Manifest().Layers {
		name, exists := layerMap[l.Digest.String()]
		if !exists {
			log.Warnf(ctx, "No mapping found for layer %s", l.Digest)

			name = artifactName(l.Annotations)
			if name == "" {
				log.Warnf(ctx, "Unable to find name for artifact layer which makes it not mountable")
				continue
			}
		}

		// Sanitize the name to prevent path traversal attacks
		safeName, err := sanitizePath("", name)
		if err != nil {
			log.Warnf(ctx, "Skipping layer with invalid name %s: %v", name, err)

			continue
		}

		if safeName == "" {
			// Multi-file/folder artifact: mount the extraction directory as the artifact root
			mountPaths = append(mountPaths, BlobMountPath{
				SourcePath:  extractDir,
				Name:        "",
				cleanupFunc: cleanupFunc,
				cleanupOnce: cleanupOnce,
			})

			continue
		}

		filePath := filepath.Join(extractDir, safeName)
		// Check if this is a single-file artifact (directory containing a file with the same name)
		sourcePath := filePath
		mountName := safeName
		if info, err := os.Stat(filePath); err == nil && info.IsDir() {
			if entries, err := os.ReadDir(filePath); err == nil && len(entries) == 1 {
				entry := entries[0]
				if !entry.IsDir() && entry.Name() == info.Name() {
					// Single-file artifact: directory X contains file X
					sourcePath = filepath.Join(filePath, entry.Name())
					mountName = entry.Name()
					log.Debugf(ctx, "Single-file artifact detected: mounting %s as %s", sourcePath, mountName)
				}
			}
		}

		mountPaths = append(mountPaths, BlobMountPath{
			SourcePath:  sourcePath,
			Name:        mountName,
			cleanupFunc: cleanupFunc,
			cleanupOnce: cleanupOnce,
		})
	}
	// Disable cleanup on success.
	// mountArtifact() will handle the cleanup.
	cleanup = false

	for _, mountPath := range mountPaths {
		log.Debugf(ctx, "Mounting artifact: SourcePath=%s, Name=%s", mountPath.SourcePath, mountPath.Name)
	}

	return mountPaths, nil
}

// extractArtifactLayers extracts tar/tar.gz layers from the pulled OCI artifact.
func (s *Store) extractArtifactLayers(ctx context.Context, _ string, parsedManifest manifest.Manifest, ref types.ImageReference, extractDir string) error {
	// Creates top-level extraction directory.
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return fmt.Errorf("failed to create artifact dir: %w", err)
	}

	// Cleanup directory if extraction fails
	var success bool
	defer func() {
		if !success {
			if err := os.RemoveAll(extractDir); err != nil {
				log.Warnf(ctx, "Failed to remove artifact extract directory on extraction failure %s: %v", extractDir, err)
			} else {
				log.Debugf(ctx, "Successfully removed artifact extract directory on extraction failure: %s", extractDir)
			}
		}
	}()

	src, err := ref.NewImageSource(ctx, s.systemContext)
	if err != nil {
		return fmt.Errorf("failed to get an image source: %w", err)
	}
	defer src.Close()

	bic := blobinfocache.DefaultCache(s.systemContext)
	layerMap := make(map[string]string)

	for _, layer := range s.impl.LayerInfos(parsedManifest) {
		err = func() error {
			blob, _, err := s.impl.GetBlob(ctx, src, types.BlobInfo{Digest: layer.Digest}, bic)
			if err != nil {
				return fmt.Errorf("failed to get blob: %w", err)
			}
			defer blob.Close()

			content, filename, err := s.processLayerContent(ctx, layer.MediaType, blob)
			if err != nil {
				return fmt.Errorf("failed to process layer: %w", err)
			}

			name := filename
			if name == "" {
				name = artifactName(layer.Annotations)
			}

			// Check if this is a tar-based layer (content is JSON-serialized files)
			if strings.Contains(layer.MediaType, ".tar") {
				var files map[string]FileInfo
				if err := json.Unmarshal(content, &files); err != nil {
					return fmt.Errorf("failed to unmarshal files from layer: %w", err)
				}

				// Find top-level regular files (not directories or symlinks)
				topFiles := make([]string, 0, len(files))

				for filePath, fileInfo := range files {
					if strings.HasPrefix(filePath, "SYMLINK:") {
						continue
					}

					if !fileInfo.IsDir && !fileInfo.IsSymlink && !strings.Contains(filePath, "/") {
						topFiles = append(topFiles, filePath)
					}
				}

				if len(topFiles) == 1 {
					// Single-file tar: use the file name and extract at root
					name = topFiles[0]
					// Double-check that the name is safe (should already be sanitized)
					safeName, err := sanitizePath("", name)
					if err != nil {
						return fmt.Errorf("invalid file name in tar: %w", err)
					}
					name = safeName
					layerMap[layer.Digest.String()] = name
					fileInfo := files[name]
					filePath := filepath.Join(extractDir, name)

					if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
						return fmt.Errorf("failed to create parent directories for %s: %w", filePath, err)
					}
					// Use the original file mode, defaulting to 0o644 if not set
					mode := fileInfo.Mode
					if mode == 0 {
						mode = 0o644
					}
					// Strip executable permissions for security
					mode &^= 0o111
					if err := os.WriteFile(filePath, fileInfo.Content, os.FileMode(mode)); err != nil {
						log.Errorf(ctx, "Failed to write file: %v", err)
						os.Remove(filePath)

						return err
					}
				} else {
					// Multi-file/folder tar: mount the entire extraction directory
					layerMap[layer.Digest.String()] = ""

					for filePath, fileInfo := range files {
						if strings.HasPrefix(filePath, "SYMLINK:") {
							actualPath := strings.TrimPrefix(filePath, "SYMLINK:")
							fullPath := filepath.Join(extractDir, actualPath)
							if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
								return fmt.Errorf("failed to create parent directories for %s: %w", fullPath, err)
							}

							// Fix symlink target to be relative to the symlink's location
							linkTarget := fileInfo.LinkTarget
							if !filepath.IsAbs(linkTarget) && !strings.HasPrefix(linkTarget, "../") && !strings.HasPrefix(linkTarget, "./") {
								// If the target is a simple filename, make it relative to the symlink's directory
								symlinkDir := filepath.Dir(actualPath)
								if symlinkDir != "." {
									linkTarget = filepath.Join("..", linkTarget)
								}
							}

							if err := os.Symlink(linkTarget, fullPath); err != nil {
								return fmt.Errorf("failed to create symlink %s -> %s: %w", fullPath, linkTarget, err)
							}

							continue
						}

						fullPath := filepath.Join(extractDir, filePath)

						if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
							return fmt.Errorf("failed to create parent directories for %s: %w", fullPath, err)
						}
						if fileInfo.IsDir {
							mode := fileInfo.Mode
							if mode == 0 {
								mode = 0o755
							}
							if err := os.MkdirAll(fullPath, os.FileMode(mode)); err != nil {
								return fmt.Errorf("failed to create directory %s: %w", fullPath, err)
							}
						} else {
							mode := fileInfo.Mode
							if mode == 0 {
								mode = 0o644
							}

							// Strip executable permissions for security
							mode &^= 0o111

							if err := os.WriteFile(fullPath, fileInfo.Content, os.FileMode(mode)); err != nil {
								log.Errorf(ctx, "Failed to write file: %v", err)
								os.Remove(fullPath)

								return err
							}
						}
					}
				}
			} else {
				// Non-tar layer - treat as single file
				// Sanitize the name to prevent path traversal attacks
				safeName, err := sanitizePath("", name)
				if err != nil {
					return fmt.Errorf("invalid file name: %w", err)
				}
				name = safeName
				layerMap[layer.Digest.String()] = name
				filePath := filepath.Join(extractDir, name)

				if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
					return fmt.Errorf("failed to create parent directories for %s: %w", filePath, err)
				}
				if err := os.WriteFile(filePath, content, 0o644); err != nil {
					log.Errorf(ctx, "Failed to write file: %v", err)
					os.Remove(filePath)
					return err
				}
			}

			return nil
		}()
		// If any layer extraction fails, abort the entire operation.
		if err != nil {
			return fmt.Errorf("failed to extract artifact layer: %w", err)
		}
	}

	// Mark extraction as successful
	success = true

	// Save layer mapping
	mapPath := filepath.Join(s.extractArtifactDir, "layer-map.json")
	data, err := json.Marshal(layerMap)

	if err != nil {
		return fmt.Errorf("failed to marshal layer map: %w", err)
	}

	if err := os.WriteFile(mapPath, data, 0o644); err != nil {
		log.Errorf(ctx, "Failed to write file: %v", err)
	}

	success = true

	return nil
}

// processLayerContent decompresses and processes layer content based on media type.
// Supports gzip, zstd, and uncompressed tar formats. Returns raw bytes for non-tar content.
// For tar formats, it extracts all files and folders and returns them as a map.
func (s *Store) processLayerContent(ctx context.Context, mediaType string, r io.Reader) (content []byte, filename string, err error) {
	switch {
	case strings.Contains(mediaType, ".tar+zstd"):
		zr, err := zstd.NewReader(r)
		if err != nil {
			return nil, "", fmt.Errorf("zstd decompress failed: %w", err)
		}
		defer zr.Close()

		files, baseDir, err := s.extractAllFromTar(ctx, zr)
		if err != nil {
			return nil, "", err
		}

		// Serialize the files map to JSON for storage
		jsonData, err := json.Marshal(files)
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal files: %w", err)
		}

		return jsonData, baseDir, nil

	case strings.Contains(mediaType, ".tar+gzip"):
		gr, err := gzip.NewReader(r)
		if err != nil {
			return nil, "", fmt.Errorf("gzip decompress failed: %w", err)
		}

		defer gr.Close()

		files, baseDir, err := s.extractAllFromTar(ctx, gr)
		if err != nil {
			return nil, "", err
		}

		// Serialize the files map to JSON for storage
		jsonData, err := json.Marshal(files)
		if err != nil {
			return nil, "", fmt.Errorf("failed to marshal files: %w", err)
		}

		return jsonData, baseDir, nil

	case strings.Contains(mediaType, ".tar"):
		// This media type can be either compressed or uncompressed
		// Try to detect compression by peeking at the first few bytes
		peekReader := io.LimitReader(r, 2)
		peekBytes, err := io.ReadAll(peekReader)
		if err != nil {
			return nil, "", fmt.Errorf("failed to peek at layer content: %w", err)
		}

		// Check for gzip magic number (0x1f 0x8b)
		if len(peekBytes) >= 2 && peekBytes[0] == 0x1f && peekBytes[1] == 0x8b {
			// Create a new reader that includes the peeked bytes
			fullReader := io.MultiReader(bytes.NewReader(peekBytes), r)
			gr, err := gzip.NewReader(fullReader)
			if err != nil {
				return nil, "", fmt.Errorf("gzip decompress failed: %w", err)
			}
			defer gr.Close()

			files, baseDir, err := s.extractAllFromTar(ctx, gr)
			if err != nil {
				return nil, "", err
			}

			// Serialize the files map to JSON for storage
			jsonData, err := json.Marshal(files)
			if err != nil {
				return nil, "", fmt.Errorf("failed to marshal files: %w", err)
			}

			return jsonData, baseDir, nil
		} else {
			// Create a new reader that includes the peeked bytes
			fullReader := io.MultiReader(bytes.NewReader(peekBytes), r)
			files, baseDir, err := s.extractAllFromTar(ctx, fullReader)
			if err != nil {
				return nil, "", err
			}

			// Serialize the files map to JSON for storage
			jsonData, err := json.Marshal(files)
			if err != nil {
				return nil, "", fmt.Errorf("failed to marshal files: %w", err)
			}

			return jsonData, baseDir, nil
		}

	default:
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, "", fmt.Errorf("reading blob content: %w", err)
		}

		return data, "", nil
	}
}

// FileInfo stores file content and metadata.
type FileInfo struct {
	Content    []byte `json:"content"`
	Mode       int64  `json:"mode"`
	IsDir      bool   `json:"is_dir"`
	IsSymlink  bool   `json:"is_symlink"`
	LinkTarget string `json:"link_target,omitempty"`
}

// extractAllFromTar extracts all files and folders from a tar archive.
// Returns a map of file paths to their FileInfo, and the base directory name if present.
func (s *Store) extractAllFromTar(ctx context.Context, r io.Reader) (files map[string]FileInfo, baseDir string, err error) {
	tr := tar.NewReader(r)
	files = make(map[string]FileInfo)

	// Track the first directory to use as base directory name
	var firstDir string

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			log.Errorf(ctx, "Error reading tar header: %v", err)
			return nil, "", fmt.Errorf("reading tar header: %w", err)
		}

		// Validate path is safe
		safePath, err := sanitizePath("", hdr.Name)
		if err != nil {
			log.Warnf(ctx, "Skipping file with invalid path %s: %v", hdr.Name, err)
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeReg:
			// Regular file
			content, err := io.ReadAll(tr)
			if err != nil {
				return nil, "", fmt.Errorf("reading file content from tar: %w", err)
			}

			files[safePath] = FileInfo{
				Content:   content,
				Mode:      hdr.Mode,
				IsDir:     false,
				IsSymlink: false,
			}

		case tar.TypeDir:
			// Directory - track the first one as base directory
			if firstDir == "" {
				firstDir = filepath.Base(safePath)
			}
			// Create empty entry to mark directory existence
			files[safePath] = FileInfo{
				Content:   nil,
				Mode:      hdr.Mode,
				IsDir:     true,
				IsSymlink: false,
			}

		case tar.TypeSymlink:
			// Symlink - store the link target
			linkTarget := hdr.Linkname
			if _, err := sanitizePath("", linkTarget); err != nil {
				log.Warnf(ctx, "Skipping symlink with invalid target %s: %v", linkTarget, err)
				continue
			}
			// Use a special key format to distinguish symlinks
			symlinkKey := "SYMLINK:" + safePath
			files[symlinkKey] = FileInfo{
				Content:    nil,
				Mode:       hdr.Mode,
				IsDir:      false,
				IsSymlink:  true,
				LinkTarget: linkTarget,
			}

		default:
			// Skip other file types (hardlinks, devices, etc.)
			// Still need to read the content to advance the reader
			if _, err := io.Copy(io.Discard, tr); err != nil {
				return nil, "", fmt.Errorf("skipping file content: %w", err)
			}
		}
	}

	if len(files) == 0 {
		return nil, "", errors.New("no files found in tar archive")
	}

	return files, firstDir, nil
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

// sanitizePath prevents path traversal attacks by ensuring the path is safe.
func sanitizePath(base, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}

	// Normalize the path
	cleanedPath := filepath.Clean(path)

	// Check for path traversal patterns
	if strings.Contains(cleanedPath, "..") {
		return "", fmt.Errorf("path traversal attack detected: %s", path)
	}

	// Check for absolute paths
	if filepath.IsAbs(cleanedPath) {
		return "", fmt.Errorf("absolute path not allowed: %s", path)
	}

	// Join with base directory if provided
	if base != "" {
		target := filepath.Join(base, cleanedPath)
		// Verify the path stays within the base directory
		if !strings.HasPrefix(target, base) {
			return "", fmt.Errorf("path escapes base directory: %s", path)
		}
		return target, nil
	}

	return cleanedPath, nil
}
