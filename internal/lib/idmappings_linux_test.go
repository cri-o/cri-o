package lib

import (
	"testing"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func TestConvertCRIToOCIMappings(t *testing.T) {
	cases := []struct {
		name     string
		input    []*types.IDMapping
		expected []rspec.LinuxIDMapping
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty input",
			input:    []*types.IDMapping{},
			expected: nil,
		},
		{
			name: "single mapping",
			input: []*types.IDMapping{
				{ContainerId: 0, HostId: 1000, Length: 65536},
			},
			expected: []rspec.LinuxIDMapping{
				{ContainerID: 0, HostID: 1000, Size: 65536},
			},
		},
		{
			name: "multiple mappings",
			input: []*types.IDMapping{
				{ContainerId: 0, HostId: 1000, Length: 1},
				{ContainerId: 1, HostId: 100000, Length: 65536},
			},
			expected: []rspec.LinuxIDMapping{
				{ContainerID: 0, HostID: 1000, Size: 1},
				{ContainerID: 1, HostID: 100000, Size: 65536},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ConvertCRIToOCIMappings(tc.input)

			if len(result) != len(tc.expected) {
				t.Fatalf("Expected %d mappings, got %d", len(tc.expected), len(result))
			}

			for i, mapping := range result {
				if mapping.ContainerID != tc.expected[i].ContainerID {
					t.Errorf("Mapping %d: expected ContainerID %d, got %d", i, tc.expected[i].ContainerID, mapping.ContainerID)
				}

				if mapping.HostID != tc.expected[i].HostID {
					t.Errorf("Mapping %d: expected HostID %d, got %d", i, tc.expected[i].HostID, mapping.HostID)
				}

				if mapping.Size != tc.expected[i].Size {
					t.Errorf("Mapping %d: expected Size %d, got %d", i, tc.expected[i].Size, mapping.Size)
				}
			}
		})
	}
}

func TestConvertOCIToStorageIDMappings(t *testing.T) {
	cases := []struct {
		name        string
		uidMappings []rspec.LinuxIDMapping
		gidMappings []rspec.LinuxIDMapping
		expectNil   bool
	}{
		{
			name:        "nil inputs",
			uidMappings: nil,
			gidMappings: nil,
			expectNil:   true,
		},
		{
			name:        "empty UID mappings",
			uidMappings: []rspec.LinuxIDMapping{},
			gidMappings: []rspec.LinuxIDMapping{{ContainerID: 0, HostID: 1000, Size: 65536}},
			expectNil:   true,
		},
		{
			name:        "empty GID mappings",
			uidMappings: []rspec.LinuxIDMapping{{ContainerID: 0, HostID: 1000, Size: 65536}},
			gidMappings: []rspec.LinuxIDMapping{},
			expectNil:   true,
		},
		{
			name: "valid single mapping",
			uidMappings: []rspec.LinuxIDMapping{
				{ContainerID: 0, HostID: 1000, Size: 65536},
			},
			gidMappings: []rspec.LinuxIDMapping{
				{ContainerID: 0, HostID: 1000, Size: 65536},
			},
			expectNil: false,
		},
		{
			name: "valid multiple mappings",
			uidMappings: []rspec.LinuxIDMapping{
				{ContainerID: 0, HostID: 1000, Size: 1},
				{ContainerID: 1, HostID: 100000, Size: 65536},
			},
			gidMappings: []rspec.LinuxIDMapping{
				{ContainerID: 0, HostID: 2000, Size: 1},
				{ContainerID: 1, HostID: 200000, Size: 65536},
			},
			expectNil: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ConvertOCIToStorageIDMappings(tc.uidMappings, tc.gidMappings)

			if tc.expectNil {
				if result != nil {
					t.Fatalf("Expected nil result, got non-nil")
				}

				return
			}

			if result == nil {
				t.Fatalf("Expected non-nil result, got nil")
			}

			uids := result.UIDs()
			gids := result.GIDs()

			if len(uids) != len(tc.uidMappings) {
				t.Fatalf("Expected %d UID mappings, got %d", len(tc.uidMappings), len(uids))
			}

			if len(gids) != len(tc.gidMappings) {
				t.Fatalf("Expected %d GID mappings, got %d", len(tc.gidMappings), len(gids))
			}

			for i, uid := range uids {
				if uid.ContainerID != int(tc.uidMappings[i].ContainerID) {
					t.Errorf("UID mapping %d: expected ContainerID %d, got %d", i, tc.uidMappings[i].ContainerID, uid.ContainerID)
				}

				if uid.HostID != int(tc.uidMappings[i].HostID) {
					t.Errorf("UID mapping %d: expected HostID %d, got %d", i, tc.uidMappings[i].HostID, uid.HostID)
				}

				if uid.Size != int(tc.uidMappings[i].Size) {
					t.Errorf("UID mapping %d: expected Size %d, got %d", i, tc.uidMappings[i].Size, uid.Size)
				}
			}

			for i, gid := range gids {
				if gid.ContainerID != int(tc.gidMappings[i].ContainerID) {
					t.Errorf("GID mapping %d: expected ContainerID %d, got %d", i, tc.gidMappings[i].ContainerID, gid.ContainerID)
				}

				if gid.HostID != int(tc.gidMappings[i].HostID) {
					t.Errorf("GID mapping %d: expected HostID %d, got %d", i, tc.gidMappings[i].HostID, gid.HostID)
				}

				if gid.Size != int(tc.gidMappings[i].Size) {
					t.Errorf("GID mapping %d: expected Size %d, got %d", i, tc.gidMappings[i].Size, gid.Size)
				}
			}
		})
	}
}
