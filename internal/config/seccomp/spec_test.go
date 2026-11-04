package seccomp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/internal/config/seccomp"
)

// TestSaveSpecToFileFlags checks that a saved spec tells a seccomp flags list
// that is set and empty apart from one that is not set, which decides whether
// the runtime sets SECCOMP_FILTER_FLAG_SPEC_ALLOW, and that the list survives
// loading and saving the spec again, as restoring a container does.
func TestSaveSpecToFileFlags(t *testing.T) {
	errnoRet := uint(1)

	for name, tc := range map[string]struct {
		flags    []rspec.LinuxSeccompFlag
		expected string
	}{
		"not set": {flags: nil},
		"empty":   {flags: []rspec.LinuxSeccompFlag{}, expected: `"flags": []`},
		"set": {
			flags:    []rspec.LinuxSeccompFlag{rspec.LinuxSeccompFlagLog},
			expected: `"SECCOMP_FILTER_FLAG_LOG"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			g, err := generate.New("linux")
			if err != nil {
				t.Fatal(err)
			}

			g.Config.Linux.Seccomp = &rspec.LinuxSeccomp{
				DefaultAction:    rspec.ActErrno,
				DefaultErrnoRet:  &errnoRet,
				Architectures:    []rspec.Arch{rspec.ArchX86_64},
				Flags:            tc.flags,
				ListenerPath:     "/run/listener.sock",
				ListenerMetadata: "metadata",
				Syscalls: []rspec.LinuxSyscall{
					{
						Names:  []string{"clone"},
						Action: rspec.ActAllow,
						// A value that does not fit a float64.
						Args: []rspec.LinuxSeccompArg{
							{Index: 0, Value: 1<<63 + 1, Op: rspec.OpMaskedEqual},
						},
					},
				},
			}

			path := filepath.Join(t.TempDir(), "config.json")
			spec := g.Config

			// The second round saves the spec as it was loaded.
			for range 2 {
				if err := seccomp.SaveSpecToFile(&g, path); err != nil {
					t.Fatal(err)
				}

				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}

				if got := strings.Contains(string(data), `"flags"`); got != (tc.expected != "") {
					t.Fatalf(
						"expected flags in the saved spec: %t, got: %t",
						tc.expected != "",
						got,
					)
				}

				if !strings.Contains(string(data), tc.expected) {
					t.Fatalf("expected %s in the saved spec, got: %s", tc.expected, data)
				}

				loaded := &rspec.Spec{}
				if err := json.Unmarshal(data, loaded); err != nil {
					t.Fatal(err)
				}

				if (loaded.Linux.Seccomp.Flags == nil) != (tc.flags == nil) {
					t.Fatalf("expected flags %#v, got %#v", tc.flags, loaded.Linux.Seccomp.Flags)
				}

				if !reflect.DeepEqual(spec, loaded) {
					t.Fatalf(
						"expected the saved spec to be the one of the generator, got: %s",
						data,
					)
				}

				if g, err = generate.NewFromFile(path); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
