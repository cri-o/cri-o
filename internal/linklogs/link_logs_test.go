package linklogs

import (
	"context"
	"strings"
	"testing"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestLogOperationsRejectInvalidVolumes(t *testing.T) {
	t.Parallel()

	for _, volume := range []string{"", "../other", "invalid name", "/logs", "logs/child"} {
		t.Run(volume, func(t *testing.T) {
			t.Parallel()

			operations := map[string]func(context.Context) error{
				"mount": func(ctx context.Context) error {
					return MountPodLogs(ctx, "pod-uid", volume, "default", "pod", "")
				},
				"link": func(ctx context.Context) error {
					return LinkContainerLogs(
						ctx,
						"pod-uid",
						volume,
						"container-id",
						&types.ContainerMetadata{},
					)
				},
				"unmount": func(ctx context.Context) error {
					return UnmountPodLogs(ctx, "pod-uid", volume)
				},
			}
			for name, operation := range operations {
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					err := operation(t.Context())
					if err == nil ||
						!strings.Contains(err.Error(), "empty dir vol name is invalid") {
						t.Fatalf(
							"invalid volume %q must be rejected before accessing paths: %v",
							volume,
							err,
						)
					}
				})
			}
		})
	}
}
