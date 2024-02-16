package seccompociartifact

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/containers/image/v5/types"

	"github.com/cri-o/cri-o/internal/config/ociartifact"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/pkg/annotations"
)

// SeccompOCIArtifact is the main structure for handling seccomp related OCI
// artifacts.
type SeccompOCIArtifact struct {
	ociArtifactImpl ociartifact.Impl
}

// New creates a new seccomp OCI artifact handler.
func New() *SeccompOCIArtifact {
	return &SeccompOCIArtifact{
		ociArtifactImpl: ociartifact.New(),
	}
}

// SeccompProfilePodAnnotation is the annotation used for matching a whole pod
// rather than a specific container.
const SeccompProfilePodAnnotation = annotations.SeccompProfileAnnotation + "/POD"

// TryPull tries to pull the OCI artifact seccomp profile while evaluating
// the provided annotations.
func (s *SeccompOCIArtifact) TryPull(
	ctx context.Context,
	sys *types.SystemContext,
	containerName string,
	podAnnotations, imageAnnotations map[string]string,
) (profile []byte, err error) {
	log.Debugf(ctx, "Evaluating seccomp annotations")

	profileRef := ""
	containerKey := fmt.Sprintf("%s/%s", annotations.SeccompProfileAnnotation, containerName)
	if val, ok := podAnnotations[containerKey]; ok {
		log.Infof(ctx, "Found container specific seccomp profile annotation: %s=%s", containerKey, val)
		profileRef = val
	} else if val, ok := podAnnotations[SeccompProfilePodAnnotation]; ok {
		log.Infof(ctx, "Found pod specific seccomp profile annotation: %s=%s", annotations.SeccompProfileAnnotation, val)
		profileRef = val
	} else if val, ok := imageAnnotations[containerKey]; ok {
		log.Infof(ctx, "Found image specific seccomp profile annotation for container %s: %s=%s", containerName, annotations.SeccompProfileAnnotation, val)
		profileRef = val
	} else if val, ok := imageAnnotations[SeccompProfilePodAnnotation]; ok {
		log.Infof(ctx, "Found image specific seccomp profile annotation for pod: %s=%s", annotations.SeccompProfileAnnotation, val)
		profileRef = val
	}

	if profileRef == "" {
		return nil, nil
	}

	artifact, err := s.ociArtifactImpl.Pull(ctx, sys, profileRef)
	if err != nil {
		return nil, fmt.Errorf("pull OCI artifact: %w", err)
	}
	defer artifact.Cleanup()

	const jsonExt = ".json"
	seccompProfilePath := ""
	if err := filepath.Walk(artifact.MountPath,
		func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() ||
				info.Mode()&os.ModeSymlink == os.ModeSymlink ||
				filepath.Ext(info.Name()) != jsonExt {
				return nil
			}

			seccompProfilePath = p

			// TODO(sgrunert): allow merging profiles, not just choosing the first one
			return fs.SkipAll
		}); err != nil {
		return nil, fmt.Errorf("walk %s: %w", artifact.MountPath, err)
	}

	log.Infof(ctx, "Trying to read profile from: %s", seccompProfilePath)
	profileContent, err := os.ReadFile(seccompProfilePath)
	if err != nil {
		return nil, fmt.Errorf("read %s from file store: %w", seccompProfilePath, err)
	}

	log.Infof(ctx, "Retrieved OCI artifact seccomp profile of len: %d", len(profileContent))
	return profileContent, nil
}
