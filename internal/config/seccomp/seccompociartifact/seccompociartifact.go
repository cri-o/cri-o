package seccompociartifact

import (
	"context"
	"errors"
	"fmt"

	"github.com/containers/image/v5/types"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/ociartifact"
	"github.com/cri-o/cri-o/pkg/annotations"
)

// SeccompOCIArtifact is the main structure for handling seccomp related OCI
// artifacts.
type SeccompOCIArtifact struct {
	impl Impl
}

// New creates a new seccomp OCI artifact handler.
func New(root string, systemContext *types.SystemContext) *SeccompOCIArtifact {
	return &SeccompOCIArtifact{
		impl: ociartifact.NewStore(root, systemContext),
	}
}

const (
	// SeccompProfilePodAnnotation is the annotation used for matching a whole pod
	// rather than a specific container.
	SeccompProfilePodAnnotation = annotations.SeccompProfileAnnotation + "/POD"

	// requiredConfigMediaType is the config media type for OCI artifact seccomp profiles.
	requiredConfigMediaType = "application/vnd.cncf.seccomp-profile.config.v1+json"
)

// TryPull tries to pull the OCI artifact seccomp profile while evaluating
// the provided annotations.
func (s *SeccompOCIArtifact) TryPull(
	ctx context.Context,
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
	} else if val, ok := imageAnnotations[annotations.SeccompProfileAnnotation]; ok {
		log.Infof(ctx, "Found image specific seccomp profile annotation: %s=%s", annotations.SeccompProfileAnnotation, val)
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

	artifactData, err := s.impl.PullData(ctx, profileRef, &ociartifact.PullOptions{EnforceConfigMediaType: requiredConfigMediaType})
	if err != nil {
		return nil, fmt.Errorf("pull OCI artifact: %w", err)
	}

	if len(artifactData) == 0 {
		return nil, errors.New("artifact data is empty")
	}

	profileData := artifactData[0].Data()
	log.Infof(ctx, "Retrieved OCI artifact seccomp profile of len: %d", len(profileData))

	return profileData, nil
}
