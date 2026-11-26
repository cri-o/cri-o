//go:build seccomp && linux && cgo

package seccomp

import (
	"strings"
	"testing"

	"github.com/opencontainers/runtime-tools/generate"

	v2 "github.com/cri-o/cri-o/pkg/annotations/v2"
)

// TestApplyProfileFromBytesNullEntries checks that a profile with a null
// syscall entry or condition is refused rather than rendered when applied,
// as a Localhost profile or one pulled as an OCI artifact is.
func TestApplyProfileFromBytesNullEntries(t *testing.T) {
	for name, tc := range map[string]struct {
		profile  string
		expected string
	}{
		"null entry": {
			`{"defaultAction": "SCMP_ACT_ERRNO", "syscalls": [null]}`,
			"syscalls[0]: null entry",
		},
		"null condition": {
			`{"defaultAction": "SCMP_ACT_ERRNO", "syscalls": [` +
				`{"names": ["read"], "action": "SCMP_ACT_ALLOW"}, ` +
				`{"names": ["write"], "action": "SCMP_ACT_ALLOW", "args": [{"index": 0, "op": "SCMP_CMP_EQ"}, null]}]}`,
			"syscalls[1].args[1]: null condition",
		},
	} {
		t.Run(name, func(t *testing.T) {
			generator, err := generate.New("linux")
			if err != nil {
				t.Fatal(err)
			}

			_, err = New().applyProfileFromBytes(
				t.Context(), []byte(tc.profile), nil, "", nil, &generator,
			)
			if err == nil || !strings.Contains(err.Error(), tc.expected) {
				t.Fatalf("expected an error containing %q, got %v", tc.expected, err)
			}
		})
	}
}

// TestApplyProfileFromBytesWithoutFilter checks that a profile without a
// filter runs the container unconfined when the notifier is requested.
func TestApplyProfileFromBytesWithoutFilter(t *testing.T) {
	for _, profile := range []string{`{}`, `null`, `{"syscalls": []}`} {
		t.Run(profile, func(t *testing.T) {
			generator, err := generate.New("linux")
			if err != nil {
				t.Fatal(err)
			}

			notifier, err := New().applyProfileFromBytes(
				t.Context(),
				[]byte(profile),
				make(chan Notification),
				"ctr",
				map[string]string{v2.SeccompNotifierAction: "stop"},
				&generator,
			)
			if err != nil {
				t.Fatal(err)
			}

			if notifier != nil || generator.Config.Linux.Seccomp != nil {
				t.Fatalf("expected no notifier and no filter, got %v and %v",
					notifier, generator.Config.Linux.Seccomp)
			}
		})
	}
}
