package oci

import (
	"encoding/json"
	"testing"
)

func TestContainerRestoreStateJSONRoundTrip(t *testing.T) {
	state := &ContainerState{
		Restore:        true,
		RestoreArchive: "/var/lib/containers/storage/checkpoint.tar",
		Restored:       true,
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}

	loaded := new(ContainerState)
	if err := json.Unmarshal(data, loaded); err != nil {
		t.Fatal(err)
	}

	if !loaded.Restore || loaded.RestoreArchive != state.RestoreArchive || !loaded.Restored {
		t.Fatalf("restore state did not survive JSON round trip: %#v", loaded)
	}
}
