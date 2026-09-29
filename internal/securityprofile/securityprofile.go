// Package securityprofile implements the security profiles distributed as OCI
// artifacts (KEP-6061): the PullSecurityProfile validation, the lookup of
// pulled profiles, and their merge with the node-local profiles.
package securityprofile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"

	ispec "github.com/opencontainers/image-spec/specs-go/v1"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/manifest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	crierrors "k8s.io/cri-api/pkg/errors"
	"sigs.k8s.io/security-profiles-merger/seccomp"

	"github.com/cri-o/cri-o/internal/storage"
)

const (
	// SeccompConfigMediaType is the config media type of a seccomp profile
	// artifact.
	SeccompConfigMediaType = "application/vnd.cncf.seccomp-profile.config.v1+json"

	// AppArmorConfigMediaType is the config media type of an AppArmor profile
	// artifact.
	AppArmorConfigMediaType = "application/vnd.cncf.apparmor-profile.config.v1+json"
)

// invalidf returns a permanent rejection of a profile, which the kubelet
// matches by the prefix of the error message and does not retry.
func invalidf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", crierrors.ErrSecurityProfileInvalid, fmt.Sprintf(format, args...))
}

// invalidRequestf returns a permanent rejection of a request, which is also
// an invalid argument.
func invalidRequestf(format string, args ...any) error {
	return status.Errorf(
		codes.InvalidArgument,
		"%s: %s",
		crierrors.ErrSecurityProfileInvalid,
		fmt.Sprintf(format, args...),
	)
}

// mediaTypeForKind returns the config media type of a profile kind.
func mediaTypeForKind(kind types.SecurityProfileKind) (string, error) {
	switch kind {
	case types.SecurityProfileKind_Seccomp:
		return SeccompConfigMediaType, nil

	case types.SecurityProfileKind_AppArmor:
		return "", invalidRequestf("AppArmor security profiles are not supported")

	default:
		return "", invalidRequestf("unsupported security profile kind %s", kind)
	}
}

// parseReference parses the canonical, digest-pinned reference of a profile
// and returns it without a tag.
func parseReference(ref string) (reference.Canonical, error) {
	// Unlike a normalized one, a canonical reference names its registry, so
	// no short-name resolution applies.
	named, err := reference.ParseNamed(ref)
	if err != nil {
		return nil, invalidRequestf("invalid security profile reference %q: %v", ref, err)
	}

	digested, ok := named.(reference.Digested)
	if !ok {
		return nil, invalidRequestf("security profile reference %q is not pinned by digest", ref)
	}

	canonical, err := reference.WithDigest(reference.TrimNamed(named), digested.Digest())
	if err != nil {
		return nil, invalidRequestf("invalid security profile reference %q: %v", ref, err)
	}

	return canonical, nil
}

// parseManifest parses a manifest fetched from a registry.
func parseManifest(raw []byte, mimeType string) (*ispec.Manifest, error) {
	if mimeType == "" {
		mimeType = manifest.GuessMIMEType(raw)
	}

	mimeType = manifest.NormalizedMIMEType(mimeType)
	if mimeType != ispec.MediaTypeImageManifest {
		return nil, invalidf("manifest media type %q is not an OCI image manifest", mimeType)
	}

	m, err := manifest.OCI1FromManifest(raw)
	if err != nil {
		return nil, invalidf("parse manifest: %v", err)
	}

	return &m.Manifest, nil
}

// artifactType returns the media type a manifest declares for its artifact:
// the config media type, or the artifactType for an empty config.
func artifactType(m *ispec.Manifest) string {
	if m.Config.MediaType == ispec.MediaTypeEmptyJSON {
		return m.ArtifactType
	}

	return m.Config.MediaType
}

// validateManifest checks what the manifest says about a profile artifact,
// so that an artifact is rejected before any of its blobs is read.
func validateManifest(m *ispec.Manifest, mediaType string, maxSize int64) error {
	// Scanners may go by either field, so they must not disagree.
	if m.Config.MediaType != ispec.MediaTypeEmptyJSON &&
		m.ArtifactType != "" && m.ArtifactType != m.Config.MediaType {
		return invalidf(
			"artifact type %q does not match config media type %q",
			m.ArtifactType, m.Config.MediaType,
		)
	}

	if t := artifactType(m); t != mediaType {
		return invalidf(
			"artifact media type %q does not match the profile kind, expected %q",
			t,
			mediaType,
		)
	}

	if len(m.Layers) != 1 {
		return invalidf("artifact has %d layers, expected exactly one", len(m.Layers))
	}

	// The limit is on the layer. The config gets the same bound, as nothing
	// but its media type is used.
	for _, desc := range []ispec.Descriptor{m.Layers[0], m.Config} {
		// A foreign blob would be fetched from wherever its URLs point.
		if len(desc.URLs) > 0 {
			return invalidf("blob %s has external URLs, which are not supported", desc.Digest)
		}

		if desc.Size < 0 || desc.Size > maxSize {
			return invalidf(
				"blob %s has a size of %d bytes, the maximum is %d bytes",
				desc.Digest,
				desc.Size,
				maxSize,
			)
		}
	}

	return nil
}

// isArchive returns true if data starts like a gzip or zstd stream or a tar
// archive, which is how an image layer looks, not a raw profile document.
func isArchive(data []byte) bool {
	const tarMagicOffset = 257

	return bytes.HasPrefix(data, []byte{0x1f, 0x8b}) ||
		bytes.HasPrefix(data, []byte{0x28, 0xb5, 0x2f, 0xfd}) ||
		(len(data) > tarMagicOffset+5 &&
			bytes.Equal(data[tarMagicOffset:tarMagicOffset+5], []byte("ustar")))
}

// parseSeccomp decodes and validates the layer of a seccomp profile artifact.
func parseSeccomp(data []byte) (*rspec.LinuxSeccomp, error) {
	if isArchive(data) {
		return nil, invalidf("profile layer is an archive, expected the raw profile document")
	}

	profile := &rspec.LinuxSeccomp{}
	if err := seccomp.UnmarshalStrict(data, profile); err != nil {
		return nil, fmt.Errorf(
			"%w: decode seccomp profile: %w",
			crierrors.ErrSecurityProfileInvalid,
			err,
		)
	}

	if err := seccomp.ValidateArtifact(profile); err != nil {
		return nil, fmt.Errorf(
			"%w: validate seccomp profile: %w",
			crierrors.ErrSecurityProfileInvalid,
			err,
		)
	}

	return profile, nil
}

// classifyPullError maps a failed pull to the well-known errors the kubelet
// retries with backoff. Errors that match none of them are retried as well.
func classifyPullError(err error) error {
	var (
		httpErr docker.UnexpectedHTTPStatusError
		dnsErr  *net.DNSError
		netErr  net.Error
	)

	switch {
	case errors.Is(err, crierrors.ErrSecurityProfileInvalid),
		errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded),
		status.Code(err) != codes.Unknown:
		return err

	// Only failures to reach the registry, not a TLS or certificate problem
	// of the node, which *url.Error reports as a net.Error as well.
	case errors.Is(err, syscall.ECONNREFUSED),
		errors.Is(err, syscall.ECONNRESET),
		errors.Is(err, syscall.EHOSTUNREACH),
		errors.Is(err, syscall.ENETUNREACH),
		errors.Is(err, syscall.ETIMEDOUT),
		errors.As(err, &dnsErr),
		errors.As(err, &netErr) && netErr.Timeout(),
		errors.As(err, &httpErr) && httpErr.StatusCode >= 500:
		return fmt.Errorf("%w: %w", crierrors.ErrRegistryUnavailable, err)
	}

	return storage.WrapSignatureCRIErrorIfNeeded(err)
}
