package annotations

import (
	"testing"

	v2 "github.com/cri-o/cri-o/pkg/annotations/v2"
)

func TestGetAnnotationValue(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		newKey      string
		wantValue   string
		wantOk      bool
	}{
		{
			name: "V2 annotation present",
			annotations: map[string]string{
				v2.UsernsMode: "auto",
			},
			newKey:    v2.UsernsMode,
			wantValue: "auto",
			wantOk:    true,
		},
		{
			name: "V1 annotation present (fallback)",
			annotations: map[string]string{
				v2.V1UsernsMode: "host", //nolint:staticcheck // Testing deprecated V1 annotation fallback
			},
			newKey:    v2.UsernsMode,
			wantValue: "host",
			wantOk:    true,
		},
		{
			name: "Both V1 and V2 present - V2 takes precedence",
			annotations: map[string]string{
				v2.V1UsernsMode: "host", //nolint:staticcheck // Testing deprecated V1 annotation fallback
				v2.UsernsMode:   "auto",
			},
			newKey:    v2.UsernsMode,
			wantValue: "auto",
			wantOk:    true,
		},
		{
			name:        "Neither present",
			annotations: map[string]string{},
			newKey:      v2.UsernsMode,
			wantValue:   "",
			wantOk:      false,
		},
		{
			name: "DisableFIPS V2 annotation",
			annotations: map[string]string{
				v2.DisableFIPS: "true",
			},
			newKey:    v2.DisableFIPS,
			wantValue: "true",
			wantOk:    true,
		},
		{
			name: "LinkLogs fallback to V1",
			annotations: map[string]string{
				v2.V1LinkLogs: "/var/log", //nolint:staticcheck // Testing deprecated V1 annotation fallback
			},
			newKey:    v2.LinkLogs,
			wantValue: "/var/log",
			wantOk:    true,
		},
		{
			name: "Container-specific annotation V2 (dot-separated)",
			annotations: map[string]string{
				v2.UnifiedCgroup + ".mycontainer": "memory.max=1000000",
			},
			newKey:    v2.UnifiedCgroup + ".mycontainer",
			wantValue: "memory.max=1000000",
			wantOk:    true,
		},
		{
			name: "Container-specific annotation V1 fallback (dot-separated)",
			annotations: map[string]string{
				v2.V1UnifiedCgroup + ".mycontainer": "memory.max=1000000", //nolint:staticcheck // Testing deprecated V1 annotation fallback
			},
			newKey:    v2.UnifiedCgroup + ".mycontainer",
			wantValue: "memory.max=1000000",
			wantOk:    true,
		},
		{
			name: "Container-specific annotation V2 (slash-separated)",
			annotations: map[string]string{
				v2.SeccompProfile + "/mycontainer": "runtime/default",
			},
			newKey:    v2.SeccompProfile + "/mycontainer",
			wantValue: "runtime/default",
			wantOk:    true,
		},
		{
			name: "Container-specific annotation V1 fallback (slash-separated)",
			annotations: map[string]string{
				v2.V1SeccompProfile + "/mycontainer": "runtime/default", //nolint:staticcheck // Testing deprecated V1 annotation fallback
			},
			newKey:    v2.SeccompProfile + "/mycontainer",
			wantValue: "runtime/default",
			wantOk:    true,
		},
		{
			name: "Container-specific annotation both present - V2 takes precedence",
			annotations: map[string]string{
				v2.V1UnifiedCgroup + ".mycontainer": "memory.max=500000", //nolint:staticcheck // Testing deprecated V1 annotation fallback
				v2.UnifiedCgroup + ".mycontainer":   "memory.max=1000000",
			},
			newKey:    v2.UnifiedCgroup + ".mycontainer",
			wantValue: "memory.max=1000000",
			wantOk:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotOk := v2.GetAnnotationValue(tt.annotations, tt.newKey)
			if gotValue != tt.wantValue {
				t.Errorf("GetAnnotationValue() value = %v, want %v", gotValue, tt.wantValue)
			}

			if gotOk != tt.wantOk {
				t.Errorf("GetAnnotationValue() ok = %v, want %v", gotOk, tt.wantOk)
			}
		})
	}
}

func TestReverseAnnotationMigrationMap(t *testing.T) {
	// Verify all V2 annotations have a V1 equivalent
	//nolint:staticcheck // Testing deprecated V1 annotation mappings
	expectedReverseMappings := map[string]string{
		v2.UsernsMode:                v2.V1UsernsMode,
		v2.Cgroup2MountHierarchyRW:   v2.V1Cgroup2MountHierarchyRW,
		v2.UnifiedCgroup:             v2.V1UnifiedCgroup,
		v2.Spoofed:                   v2.V1Spoofed,
		v2.ShmSize:                   v2.V1ShmSize,
		v2.Devices:                   v2.V1Devices,
		v2.TrySkipVolumeSELinuxLabel: v2.V1TrySkipVolumeSELinuxLabel,
		v2.SeccompNotifierAction:     v2.V1SeccompNotifierAction,
		v2.Umask:                     v2.V1Umask,
		v2.PodLinuxOverhead:          v2.V1PodLinuxOverhead,
		v2.PodLinuxResources:         v2.V1PodLinuxResources,
		v2.LinkLogs:                  v2.V1LinkLogs,
		v2.PlatformRuntimePath:       v2.V1PlatformRuntimePath,
		v2.SeccompProfile:            v2.V1SeccompProfile,
		v2.DisableFIPS:               v2.V1DisableFIPS,
	}

	// Test that GetAnnotationValue properly falls back to V1 for each annotation
	for newKey, oldKey := range expectedReverseMappings {
		t.Run(newKey, func(t *testing.T) {
			annotations := map[string]string{
				oldKey: "test-value",
			}

			gotValue, gotOk := v2.GetAnnotationValue(annotations, newKey)
			if !gotOk {
				t.Errorf("GetAnnotationValue did not find V1 fallback for %s", newKey)
			}

			if gotValue != "test-value" {
				t.Errorf("GetAnnotationValue returned wrong value: got %s, want %s", gotValue, "test-value")
			}
		})
	}
}

func TestAllAllowedAnnotationsContainsBothVersions(t *testing.T) {
	// Create a set of all allowed annotations for easy lookup
	allowedSet := make(map[string]bool)
	for _, annotation := range AllAllowedAnnotations {
		allowedSet[annotation] = true
	}

	// Verify that both V1 and V2 versions are in the allowed list
	//nolint:staticcheck // Testing deprecated V1 annotations
	annotationsToCheck := []struct {
		v1 string
		v2 string
	}{
		{v2.V1UsernsMode, v2.UsernsMode},
		{v2.V1Cgroup2MountHierarchyRW, v2.Cgroup2MountHierarchyRW},
		{v2.V1UnifiedCgroup, v2.UnifiedCgroup},
		{v2.V1Spoofed, v2.Spoofed},
		{v2.V1ShmSize, v2.ShmSize},
		{v2.V1Devices, v2.Devices},
		{v2.V1TrySkipVolumeSELinuxLabel, v2.TrySkipVolumeSELinuxLabel},
		{v2.V1SeccompNotifierAction, v2.SeccompNotifierAction},
		{v2.V1Umask, v2.Umask},
		{v2.V1PodLinuxOverhead, v2.PodLinuxOverhead},
		{v2.V1PodLinuxResources, v2.PodLinuxResources},
		{v2.V1LinkLogs, v2.LinkLogs},
		{v2.V1PlatformRuntimePath, v2.PlatformRuntimePath},
		{v2.V1SeccompProfile, v2.SeccompProfile},
		{v2.V1DisableFIPS, v2.DisableFIPS},
	}

	for _, pair := range annotationsToCheck {
		if !allowedSet[pair.v1] {
			t.Errorf("V1 annotation %s not found in AllAllowedAnnotations", pair.v1)
		}

		if !allowedSet[pair.v2] {
			t.Errorf("V2 annotation %s not found in AllAllowedAnnotations", pair.v2)
		}
	}
}
