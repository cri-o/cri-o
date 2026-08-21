package server

import (
	"context"
	"testing"

	"github.com/opencontainers/runtime-tools/generate"

	libconfig "github.com/cri-o/cri-o/pkg/config"
)

func TestConfigureGeneratorForSysctlsUnprivilegedPort(t *testing.T) {
	t.Parallel()

	const key = "net.ipv4.ip_unprivileged_port_start"

	cases := []struct {
		description  string
		hostNetwork  bool
		inputSysctls map[string]string
		wantValue    string
		wantAbsent   bool
	}{
		{
			description:  "sets default when pod has its own network namespace",
			hostNetwork:  false,
			inputSysctls: map[string]string{},
			wantValue:    "0",
		},
		{
			description:  "does not set when pod shares host network namespace",
			hostNetwork:  true,
			inputSysctls: map[string]string{},
			wantAbsent:   true,
		},
		{
			description:  "does not override user-supplied value",
			hostNetwork:  false,
			inputSysctls: map[string]string{key: "1024"},
			wantValue:    "1024",
		},
	}

	for _, tc := range cases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()

			g, err := generate.New("linux")
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			s := &Server{
				config: libconfig.Config{},
			}

			result := s.configureGeneratorForSysctls(context.Background(), &g, tc.hostNetwork, false, nil, tc.inputSysctls)

			if tc.wantAbsent {
				if _, found := result[key]; found {
					t.Errorf("expected %s to be absent from result, got %q", key, result[key])
				}

				return
			}

			got, found := result[key]
			if !found {
				t.Errorf("expected %s=%q in result, but key was absent", key, tc.wantValue)

				return
			}

			if got != tc.wantValue {
				t.Errorf("expected %s=%q, got %q", key, tc.wantValue, got)
			}
		})
	}
}
