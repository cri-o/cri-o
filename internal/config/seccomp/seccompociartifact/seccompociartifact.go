package seccompociartifact

import (
	"context"
	"fmt"

	"github.com/containers/image/v5/types"

	"github.com/cri-o/cri-o/internal/config/ociartifact"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/pkg/annotations"
)

// SeccompOCIArtifact is the main structure for handling seccomp related OCI
// artifacts.
type SeccompOCIArtifact struct {
	impl Impl
}

// New creates a new seccomp OCI artifact handler.
func New() *SeccompOCIArtifact {
	return &SeccompOCIArtifact{
		impl: ociartifact.New(),
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

	pullOptions := &ociartifact.PullOptions{
		SystemContext:          sys,
		EnforceConfigMediaType: requiredConfigMediaType,
		CachePath:              "/var/lib/crio/seccomp-oci-artifacts",
	}
	artifact, err := s.impl.Pull(ctx, profileRef, pullOptions)
	if err != nil {
		return nil, fmt.Errorf("pull OCI artifact: %w", err)
	}

	log.Infof(ctx, "Retrieved OCI artifact seccomp profile of len: %d", len(artifact.Data))
	return artifact.Data, nil
}
