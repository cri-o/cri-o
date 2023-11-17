package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/image/v5/copy"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/signature"
	"github.com/containers/image/v5/transports/alltransports"
	"github.com/containers/image/v5/types"
	"github.com/cri-o/cri-o/internal/log"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// PullArtifact can be used to pull an OCI artifact from the specified ref to
// the destPath.
func PullArtifact(ctx context.Context, sctx *types.SystemContext, ref, destPath string) error {
	log.Debugf(ctx, "Pulling artifact from %q to %q", ref, destPath)

	tmpDir, err := os.MkdirTemp("", "cri-o-artifact-pull-")
	if err != nil {
		return fmt.Errorf("create temp artifact dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	srcRef, err := alltransports.ParseImageName("docker://" + ref)
	if err != nil {
		return fmt.Errorf("parse source reference: %w", err)
	}

	destRef, err := alltransports.ParseImageName("dir:" + tmpDir)
	if err != nil {
		return fmt.Errorf("parse destination reference: %w", err)
	}

	policy, err := signature.DefaultPolicy(sctx)
	if err != nil {
		return fmt.Errorf("get default policy: %w", err)
	}
	policyContext, err := signature.NewPolicyContext(policy)
	if err != nil {
		return fmt.Errorf("create new policy context: %w", err)
	}

	manifestBlob, err := copy.Image(ctx, policyContext, destRef, srcRef, &copy.Options{
		SourceCtx:      sctx,
		DestinationCtx: sctx,
	})
	if err != nil {
		return fmt.Errorf("copy OCI artifact image: %w", err)
	}

	mfest, err := manifest.FromBlob(manifestBlob, v1.MediaTypeImageManifest)
	if err != nil {
		return fmt.Errorf("get manifest from blob: %w", err)
	}

	for _, layerInfo := range mfest.LayerInfos() {
		title, ok := layerInfo.Annotations[v1.AnnotationTitle]
		if !ok {
			log.Debugf(ctx, "No title for layer with digest: %s", layerInfo.Digest)
			continue
		}

		log.Debugf(ctx, "Found artifact %s for digest: %s", title, layerInfo.Digest.Encoded())
		fromPath := filepath.Join(tmpDir, layerInfo.Digest.Encoded())
		toPath := filepath.Join(destPath, title)

		artifactBytes, err := os.ReadFile(fromPath)
		if err != nil {
			return fmt.Errorf("read artifact content: %w", err)
		}

		if err := os.MkdirAll(filepath.Dir(toPath), 0o755); err != nil {
			return fmt.Errorf("ensure destination path exists: %w", err)
		}

		log.Debugf(ctx, "Write artifact content to: %s", toPath)
		if err := os.WriteFile(toPath, artifactBytes, 0o644); err != nil {
			return fmt.Errorf("write artifact content: %w", err)
		}
	}

	return nil
}
