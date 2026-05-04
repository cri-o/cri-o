package server

import (
	"testing"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestEventStatusRefreshPolicy(t *testing.T) {
	tests := map[string]struct {
		eventType types.ContainerEventType
		want      bool
	}{
		"created event": {
			eventType: types.ContainerEventType_CONTAINER_CREATED_EVENT,
			want:      true,
		},
		"started event": {
			eventType: types.ContainerEventType_CONTAINER_STARTED_EVENT,
			want:      true,
		},
		"stopped event": {
			eventType: types.ContainerEventType_CONTAINER_STOPPED_EVENT,
			want:      false,
		},
		"deleted event": {
			eventType: types.ContainerEventType_CONTAINER_DELETED_EVENT,
			want:      false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := eventNeedsStatusRefresh(test.eventType)
			if got != test.want {
				t.Errorf("status refresh = %t, want %t", got, test.want)
			}
		})
	}
}
