//go:build seccomp && linux && cgo

package seccomp

import (
	"slices"
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/common/pkg/seccomp"
	spmseccomp "sigs.k8s.io/security-profiles-merger/seccomp"
)

// TestSafeShapeDefaultProfile checks that the default profile, restated by
// safeShape, survives an intersection with a profile that allows everything:
// the shapes of its clone and socket rules would otherwise deny both.
func TestSafeShapeDefaultProfile(t *testing.T) {
	caps := []string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_FSETID", "CAP_FOWNER", "CAP_SETGID", "CAP_SETUID", "CAP_SETPCAP", "CAP_NET_BIND_SERVICE", "CAP_KILL"}
	spec := &rspec.Spec{Process: &rspec.Process{Capabilities: &rspec.LinuxCapabilities{Bounding: caps, Effective: caps, Permitted: caps}}}

	rendered, err := seccomp.LoadProfileFromConfig(DefaultProfile(), spec)
	if err != nil {
		t.Fatal(err)
	}

	if err := spmseccomp.ValidateArtifact(rendered); err == nil {
		t.Fatal("expected the rendered default profile to have conflicting entries")
	}

	base, err := mergeableBase(rendered)
	if err != nil {
		t.Fatalf("unexpected error for the restated default profile: %v", err)
	}

	if err := spmseccomp.ValidateArtifact(base); err != nil {
		t.Fatalf("unexpected conflicting entries in the restated default profile: %v", err)
	}

	merged, err := spmseccomp.Intersect(base, &rspec.LinuxSeccomp{DefaultAction: rspec.ActAllow})
	if err != nil {
		t.Fatal(err)
	}

	actions := func(name string) []rspec.LinuxSeccompAction {
		var found []rspec.LinuxSeccompAction

		for _, sc := range merged.Syscalls {
			if slices.Contains(sc.Names, name) {
				found = append(found, sc.Action)
			}
		}

		return found
	}

	for name, expected := range map[string][]rspec.LinuxSeccompAction{
		"fork":   {rspec.ActAllow},
		"clone":  {rspec.ActAllow},
		"setns":  {rspec.ActAllow},
		"socket": {rspec.ActAllow},
		"clone3": nil,
	} {
		got := actions(name)
		slices.Sort(got)
		slices.Sort(expected)

		if !slices.Equal(got, expected) {
			t.Errorf("%s: expected actions %v, got %v", name, expected, got)
		}
	}
}
