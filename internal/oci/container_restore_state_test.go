package oci

import (
	"encoding/json"
	"testing"

	"github.com/cri-o/cri-o/internal/storage"
)

func TestContainerRestoreStateJSONRoundTrip(t *testing.T) {
	state := &ContainerState{
		Restore:        true,
		RestoreArchive: "/var/lib/containers/storage/checkpoint.tar",
		RestoreImage:   "2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812",
		PodRestoreKey:  "namespace/pod/uid/0",
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

	if !loaded.Restore || loaded.RestoreArchive != state.RestoreArchive ||
		loaded.RestoreImage != state.RestoreImage ||
		loaded.PodRestoreKey != state.PodRestoreKey || !loaded.Restored {
		t.Fatalf("restore state did not survive JSON round trip: %#v", loaded)
	}
}

func TestContainerRestoreImageSurvivesStateReload(t *testing.T) {
	id, err := storage.ParseStorageImageIDFromOutOfProcessData(
		"2a03a6059f21e150ae84b0973863609494aad70f0a80eaeb64bddd8d92465812",
	)
	if err != nil {
		t.Fatal(err)
	}

	saved := &Container{state: &ContainerState{}}
	saved.SetRestoreStorageImageID(&id)

	data, err := json.Marshal(saved.state)
	if err != nil {
		t.Fatal(err)
	}

	loaded := &Container{state: new(ContainerState)}
	if err := json.Unmarshal(data, loaded.state); err != nil {
		t.Fatal(err)
	}

	restored := loaded.RestoreStorageImageID()
	if restored == nil ||
		restored.IDStringForOutOfProcessConsumptionOnly() != id.IDStringForOutOfProcessConsumptionOnly() {
		t.Fatalf("restore image did not survive a state reload: %v", restored)
	}

	loaded.SetRestoreStorageImageID(nil)

	if loaded.RestoreStorageImageID() != nil {
		t.Fatal("clearing the restore image kept the saved ID")
	}
}
