package seccomp

import (
	"context"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

// OCIProfiles provides the seccomp profiles pulled with PullSecurityProfile,
// which the OCI profile type references (KEP-6061), to one sandbox or
// container.
type OCIProfiles interface {
	// MergeSeccomp intersects the profile pulled for ref with its base
	// profile, which is nil if it does not restrict anything. fromPodSpec is
	// true for a Localhost base profile of the pod spec, false for the
	// runtime default.
	MergeSeccomp(
		ctx context.Context,
		ref string,
		base *rspec.LinuxSeccomp,
		fromPodSpec bool,
	) (*rspec.LinuxSeccomp, error)
}
