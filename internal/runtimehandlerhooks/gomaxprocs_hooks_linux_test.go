package runtimehandlerhooks

import (
	"strings"
	"testing"

	"github.com/opencontainers/runtime-tools/generate"

	"github.com/cri-o/cri-o/pkg/config"
)

func TestMinInjectedGOMAXPROCS(t *testing.T) {
	cases := []struct {
		name      string
		specEnvs  []string
		maxProcs  int64
		expectSet bool
		expectVal string
	}{
		{
			name:      "injects when not set",
			specEnvs:  []string{"FOO=bar"},
			maxProcs:  4,
			expectSet: true,
			expectVal: "4",
		},
		{
			name:      "skips when set in spec envs",
			specEnvs:  []string{"GOMAXPROCS=16"},
			maxProcs:  4,
			expectSet: false,
		},
		{
			name:      "skips when set via default_env (already merged into spec)",
			specEnvs:  []string{"FOO=bar", "GOMAXPROCS=8"},
			maxProcs:  4,
			expectSet: false,
		},
		{
			name:      "skips when GOMAXPROCS prefix matches in envs",
			specEnvs:  []string{"GOMAXPROCS=0"},
			maxProcs:  4,
			expectSet: false,
		},
		{
			name:      "injects with large value",
			specEnvs:  nil,
			maxProcs:  128,
			expectSet: true,
			expectVal: "128",
		},
		{
			name:      "injects with value 1",
			specEnvs:  nil,
			maxProcs:  1,
			expectSet: true,
			expectVal: "1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := generate.New("linux")
			if err != nil {
				t.Fatalf("failed to create generator: %v", err)
			}

			// Pre-populate the spec's process env to simulate image/pod envs.
			for _, env := range tc.specEnvs {
				parts := strings.SplitN(env, "=", 2)
				g.AddProcessEnv(parts[0], parts[1])
			}

			envsBefore := len(g.Config.Process.Env)

			injectGOMAXPROCS(&g, tc.maxProcs)

			envsAfter := len(g.Config.Process.Env)
			injected := envsAfter > envsBefore

			if tc.expectSet && !injected {
				t.Error("expected GOMAXPROCS to be injected, but it was not")
			}

			if !tc.expectSet && injected {
				t.Error("expected GOMAXPROCS to not be injected, but it was")
			}

			if tc.expectSet {
				for _, env := range g.Config.Process.Env {
					if val, ok := strings.CutPrefix(env, "GOMAXPROCS="); ok {
						if val != tc.expectVal {
							t.Errorf("expected GOMAXPROCS=%s, got GOMAXPROCS=%s", tc.expectVal, val)
						}
					}
				}
			}
		})
	}
}

func TestCalculateGOMAXPROCS(t *testing.T) {
	cases := []struct {
		name             string
		shares           int64
		fallbackMaxProcs int64
		expectedMaxProcs int64
	}{
		{
			name:             "2 CPU request (shares=2048), floor=4 -> use floor",
			shares:           2048,
			fallbackMaxProcs: 4,
			expectedMaxProcs: 4,
		},
		{
			name:             "500m request (shares=512), floor=4 -> use floor",
			shares:           512,
			fallbackMaxProcs: 4,
			expectedMaxProcs: 4,
		},
		{
			name:             "8 CPU request (shares=8192), floor=4 -> use calculated",
			shares:           8192,
			fallbackMaxProcs: 4,
			expectedMaxProcs: 8,
		},
		{
			name:             "1 CPU request (shares=1024), floor=4 -> use floor",
			shares:           1024,
			fallbackMaxProcs: 4,
			expectedMaxProcs: 4,
		},
		{
			name:             "4 CPU request (shares=4096), floor=4 -> use floor (equal, not greater)",
			shares:           4096,
			fallbackMaxProcs: 4,
			expectedMaxProcs: 4,
		},
		{
			name:             "16 CPU request (shares=16384), floor=4 -> use calculated",
			shares:           16384,
			fallbackMaxProcs: 4,
			expectedMaxProcs: 16,
		},
		{
			name:             "best-effort (shares=2), floor=4 -> use floor",
			shares:           2,
			fallbackMaxProcs: 4,
			expectedMaxProcs: 4,
		},
		{
			name:             "100m request (shares=102), floor=4 -> use floor",
			shares:           102,
			fallbackMaxProcs: 4,
			expectedMaxProcs: 4,
		},
		{
			name:             "5 CPU request (shares=5120), floor=2 -> use calculated",
			shares:           5120,
			fallbackMaxProcs: 2,
			expectedMaxProcs: 5,
		},
		{
			name:             "250m request (shares=256), floor=1 -> use floor",
			shares:           256,
			fallbackMaxProcs: 1,
			expectedMaxProcs: 1,
		},
		{
			name:             "3 CPU request (shares=3072), floor=2 -> use calculated",
			shares:           3072,
			fallbackMaxProcs: 2,
			expectedMaxProcs: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateGOMAXPROCS(tc.shares, tc.fallbackMaxProcs)

			if got != tc.expectedMaxProcs {
				t.Errorf("expected GOMAXPROCS=%d, got %d (shares=%d, floor=%d)",
					tc.expectedMaxProcs, got, tc.shares, tc.fallbackMaxProcs)
			}
		})
	}
}

func TestIsWorkloadPartitioned(t *testing.T) {
	workloads := config.Workloads{
		"management": &config.WorkloadConfig{
			ActivationAnnotation: "target.workload.openshift.io/management",
			AnnotationPrefix:     "resources.workload.openshift.io",
			Resources: &config.Resources{
				CPUShares: 0,
			},
		},
	}

	cases := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name: "workload-partitioned pod",
			annotations: map[string]string{
				"target.workload.openshift.io/management": `{"effect":"PreferredDuringScheduling"}`,
			},
			expected: true,
		},
		{
			name: "non-workload-partitioned pod",
			annotations: map[string]string{
				"some-other-annotation": "value",
			},
			expected: false,
		},
		{
			name:        "no annotations",
			annotations: map[string]string{},
			expected:    false,
		},
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    false,
		},
		{
			name: "workload annotation with other annotations",
			annotations: map[string]string{
				"target.workload.openshift.io/management": `{"effect":"PreferredDuringScheduling"}`,
				"resources.workload.openshift.io/dns":     `{"cpushares":51}`,
			},
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := workloads.IsWorkloadPartitioned(tc.annotations)
			if result != tc.expected {
				t.Errorf("expected IsWorkloadPartitioned=%v, got %v", tc.expected, result)
			}
		})
	}

	t.Run("empty workloads config", func(t *testing.T) {
		emptyWorkloads := config.Workloads{}

		annotations := map[string]string{
			"target.workload.openshift.io/management": `{"effect":"PreferredDuringScheduling"}`,
		}

		if emptyWorkloads.IsWorkloadPartitioned(annotations) {
			t.Error("expected false when no workloads are configured")
		}
	})
}
