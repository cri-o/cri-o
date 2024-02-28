package ociartifact

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache"
	"github.com/containers/image/v5/types"

	"github.com/cri-o/cri-o/internal/log"
)

// OCIArtifact is the main structure of this package.
type OCIArtifact struct {
	impl Impl
}

// New returns a new OCI artifact implementation.
func New() *OCIArtifact {
	return &OCIArtifact{
		impl: &defaultImpl{},
	}
}

// Artifact can be used to manage OCI artifacts.
type Artifact struct {
	// Data is the actual artifact content.
	Data []byte
}

// PullOptions can be used to customize the pull behavior.
type PullOptions struct {
	// SystemContext is the context used for pull.
	SystemContext *types.SystemContext

	// EnforceConfigMediaType can be set to enforce a specific manifest config media type.
	EnforceConfigMediaType string

	// MaxSize is the maximum size of the artifact to be allowed to get pulled.
	// Will be set to a default of 1MiB if not specified (zero) or below zero.
	MaxSize int
}

// defaultMaxArtifactSize is the default maximum artifact size.
const defaultMaxArtifactSize = 1024 * 1024 // 1 MiB

// Pull downloads the artifact content by using the provided image name and the specified options.
func (o *OCIArtifact) Pull(ctx context.Context, img string, opts *PullOptions) (*Artifact, error) {
	log.Infof(ctx, "Pulling OCI artifact from ref: %s", img)

	// Use default pull options
	if opts == nil {
		opts = &PullOptions{}
	}

	name, err := o.impl.ParseNormalizedNamed(img)
	if err != nil {
		return nil, fmt.Errorf("parse image name: %w", err)
	}
	name = reference.TagNameOnly(name) // make sure to add ":latest" if needed

	ref, err := o.impl.NewReference(name)
	if err != nil {
		return nil, fmt.Errorf("create docker reference: %w", err)
	}

	src, err := o.impl.NewImageSource(ctx, ref, opts.SystemContext)
	if err != nil {
		return nil, fmt.Errorf("build image source: %w", err)
	}

	manifestBytes, mimeType, err := o.impl.GetManifest(ctx, src, nil)
	if err != nil {
		return nil, fmt.Errorf("get manifest: %w", err)
	}

	parsedManifest, err := o.impl.ManifestFromBlob(manifestBytes, mimeType)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if opts.EnforceConfigMediaType != "" && o.impl.ManifestConfigInfo(parsedManifest).MediaType != opts.EnforceConfigMediaType {
		return nil, fmt.Errorf(
			"wrong config media type %q, requires %q",
			o.impl.ManifestConfigInfo(parsedManifest).MediaType,
			opts.EnforceConfigMediaType,
		)
	}

	layers := o.impl.LayerInfos(parsedManifest)
	if len(layers) < 1 {
		return nil, errors.New("artifact needs at least one layer")
	}

	// Just supporting one layer here. This can be later enhanced by extending
	// the PullOptions.
	layer := layers[0]

	bic := blobinfocache.DefaultCache(opts.SystemContext)
	rc, size, err := o.impl.GetBlob(ctx, src, layer.BlobInfo, bic)
	if err != nil {
		return nil, fmt.Errorf("get layer blob: %w", err)
	}
	defer rc.Close()

	maxArtifactSize := defaultMaxArtifactSize
	if opts.MaxSize > 0 {
		maxArtifactSize = opts.MaxSize
	}

	if size != -1 && size > int64(maxArtifactSize)+1 {
		return nil, fmt.Errorf("exceeded maximum allowed size of %d bytes", maxArtifactSize)
	}

	layerBytes, err := o.readLimit(rc, maxArtifactSize)
	if err != nil {
		return nil, fmt.Errorf("read with limit: %w", err)
	}

	if err := verifyDigest(&layer, layerBytes); err != nil {
		return nil, fmt.Errorf("verify digest of layer: %w", err)
	}

	return &Artifact{
		Data: layerBytes,
	}, nil
}

func (o *OCIArtifact) readLimit(reader io.Reader, limit int) ([]byte, error) {
	limitedReader := io.LimitReader(reader, int64(limit+1))

	res, err := o.impl.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("read from limit reader: %w", err)
	}

	if len(res) > limit {
		return nil, fmt.Errorf("exceeded maximum allowed size of %d bytes", limit)
	}

	return res, nil
}

func verifyDigest(layer *manifest.LayerInfo, layerBytes []byte) error {
	expectedDigest := layer.BlobInfo.Digest
	if err := expectedDigest.Validate(); err != nil {
		return fmt.Errorf("invalid digest %q: %w", expectedDigest, err)
	}
	digestAlgorithm := expectedDigest.Algorithm()
	digester := digestAlgorithm.Digester()

	hash := digester.Hash()
	hash.Write(layerBytes)
	sum := hash.Sum(nil)
	layerBytesHex := hex.EncodeToString(sum)
	if layerBytesHex != layer.BlobInfo.Digest.Hex() {
		return fmt.Errorf(
			"sha256 mismatch between real layer bytes (%s) and manifest descriptor (%s)",
			layerBytesHex, layer.BlobInfo.Digest.Hex(),
		)
	}

	return nil
}
