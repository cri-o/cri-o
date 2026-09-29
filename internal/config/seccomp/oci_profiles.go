package seccomp

import (
	"context"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

// OCIProfiles provides the seccomp profiles pulled with PullSecurityProfile,
// which the OCI profile type references (KEP-6061), to one sandbox or
// container.
type OCIProfiles interface {
	// MergeSeccomp intersects the profile pulled for ref with the baseline
	// and the base profile of the pod spec. Either is nil if it does not
	// restrict anything.
	MergeSeccomp(
		ctx context.Context,
		ref string,
		baseline, base *rspec.LinuxSeccomp,
	) (*rspec.LinuxSeccomp, error)
}
