package server

import (
	"context"
	"errors"
	"os"
	"time"

	imageTypes "go.podman.io/image/v5/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/securityprofile"
)

// PullSecurityProfile pulls a security profile with authentication config.
func (s *Server) PullSecurityProfile(
	ctx context.Context,
	req *types.PullSecurityProfileRequest,
) (*types.PullSecurityProfileResponse, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()

	ref := req.GetImage().GetImage()
	log.Debugf(ctx, "Pulling security profile: %s", ref)

	// Namespace specific signature policies and credentials apply as they
	// do for images.
	namespace := req.GetSandboxConfig().GetMetadata().GetNamespace()

	sourceCtx, err := s.contextForNamespace(namespace)
	if err != nil {
		return nil, err
	}

	credentials, err := credentialsFromAuth(req.GetAuth())
	if err != nil {
		log.Debugf(ctx, "Error decoding authentication for security profile %s: %v", ref, err)

		return nil, err
	}

	if credentials.Username != "" {
		sourceCtx.DockerAuthConfig = &credentials
	}

	sourceCtx.DockerLogMirrorChoice = true

	// The credential provider writes a namespaced auth file for every
	// request, which only a pull consumes.
	var auth *securityprofile.Auth
	if namespace != "" {
		auth = &securityprofile.Auth{
			Scope: namespace,
			Prepare: func(sys *imageTypes.SystemContext) (func(), error) {
				return s.prepareTempAuthFile(ctx, sys, ref, namespace)
			},
			Discard: func() { s.removeNamespacedAuthFile(ctx, ref, namespace) },
		}
	}

	// The error messages start with the well-known errors the kubelet
	// matches, so they must not get wrapped.
	cached, err := s.securityProfiles.Pull(ctx, ref, req.GetProfileKind(), &sourceCtx, auth)
	if err != nil {
		log.Warnf(ctx, "Unable to pull security profile %s: %v", ref, err)

		return nil, err
	}

	if cached {
		log.Debugf(ctx, "Security profile %s is present", ref)
	} else {
		log.Infof(ctx, "Pulled security profile %s", ref)
	}

	return &types.PullSecurityProfileResponse{Cached: cached}, nil
}

// removeNamespacedAuthFile removes the namespaced auth file of a request that
// needs no pull.
func (s *Server) removeNamespacedAuthFile(ctx context.Context, ref, namespace string) {
	path, err := s.namespacedAuthFilePath(ref, namespace)
	if err != nil {
		return
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warnf(ctx, "Unable to remove auth file %s: %v", path, err)
	}
}

const (
	// securityProfileGCInterval is how often unused security profiles are
	// removed.
	securityProfileGCInterval = 10 * time.Minute

	// securityProfileUnused is how long a security profile no sandbox or
	// container references is kept, which avoids pulling it again for a
	// pod that is recreated.
	securityProfileUnused = time.Hour
)

// collectSecurityProfiles removes the security profiles that no sandbox or
// container references and that were not used for a while.
func (s *Server) collectSecurityProfiles(ctx context.Context) {
	ticker := time.NewTicker(securityProfileGCInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.monitorsChan:
			return

		case <-ctx.Done():
			return

		case <-ticker.C:
			s.securityProfiles.GC(ctx, s.securityProfileReferences(), securityProfileUnused)
		}
	}
}

// securityProfileReferences returns the profile references of the sandboxes
// and containers, among other seccomp profile references.
func (s *Server) securityProfileReferences() []string {
	var references []string

	for _, sb := range s.ListSandboxes() {
		references = append(references, sb.SeccompProfilePath())
	}

	containers, err := s.ContainerServer.ListContainers()
	if err == nil {
		for _, ctr := range containers {
			references = append(references, ctr.SeccompProfilePath())
		}
	}

	return references
}

// validateSecurityProfiles rejects an unsupported use of the OCI profile type
// before anything is created for a sandbox or container.
func (s *Server) validateSecurityProfiles(
	seccompProfile, apparmorProfile *types.SecurityProfile,
) error {
	if apparmorProfile.GetProfileType() == types.SecurityProfile_OCI {
		return status.Error(
			codes.InvalidArgument,
			"the OCI profile type is not supported for AppArmor",
		)
	}

	if seccompProfile.GetProfileType() != types.SecurityProfile_OCI {
		return nil
	}

	if seccompProfile.GetOciRef() == "" {
		return status.Error(codes.InvalidArgument, "OCI seccomp profile without a reference")
	}

	if s.config.Seccomp().IsDisabled() {
		return status.Error(
			codes.FailedPrecondition,
			"seccomp is not enabled, cannot run with an OCI profile",
		)
	}

	return nil
}
