//go:build linux && test

/*
Copyright The CRI-O Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"
)

func makePodRestoreSecurityArchive(
	t *testing.T,
	spec *rspec.Spec,
	name string,
	data []byte,
	copies int,
) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "archive")
	require.NoError(t, err)

	writer := tar.NewWriter(file)
	config, err := json.Marshal(
		&metadata.ContainerConfig{
			ID:             "source",
			OCIRuntime:     "runc",
			RootfsImageRef: strings.Repeat("a", 64),
		},
	)
	require.NoError(t, err)
	encodedSpec, err := json.Marshal(spec)
	require.NoError(t, err)

	type archiveEntry struct {
		name string
		data []byte
	}

	entries := make([]archiveEntry, 0, 2+copies)

	entries = append(
		entries,
		archiveEntry{metadata.ConfigDumpFile, config},
		archiveEntry{metadata.SpecDumpFile, encodedSpec},
	)
	for range copies {
		entries = append(entries, archiveEntry{name, data})
	}

	for _, entry := range entries {
		require.NoError(
			t,
			writer.WriteHeader(
				&tar.Header{
					Name:     entry.name,
					Typeflag: tar.TypeReg,
					Mode:     0o600,
					Size:     int64(len(entry.data)),
				},
			),
		)
		_, err = writer.Write(entry.data)
		require.NoError(t, err)
	}

	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())

	return file.Name()
}

func TestPodRestoreStagedSecurityPolicy(t *testing.T) {
	captured := &rspec.Spec{
		Process: &rspec.Process{},
		Linux:   &rspec.Linux{Seccomp: &rspec.LinuxSeccomp{DefaultAction: rspec.ActAllow}},
	}

	for _, tightened := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "unchanged", true: "tightened"}[tightened],
			func(t *testing.T) {
				source := makePodRestoreSecurityArchive(t, captured, "", nil, 0)
				data, err := json.Marshal(captured)
				require.NoError(t, err)

				var destination rspec.Spec
				require.NoError(t, json.Unmarshal(data, &destination))

				if tightened {
					destination.Linux.Seccomp.Syscalls = []rspec.LinuxSyscall{
						{Names: []string{"kill"}, Action: rspec.ActErrno},
					}
				}

				directory := t.TempDir()

				err = stagePodRestoreSecurityArchive(
					t,
					source,
					filepath.Join(directory, "staged.tar"),
					&destination,
				)
				if tightened {
					require.ErrorContains(t, err, "resolved policy")
					entries, err := os.ReadDir(directory)
					require.NoError(t, err)
					require.Empty(t, entries)
				} else {
					require.NoError(t, err)
				}
			},
		)
	}
}

func TestPodRestoreStagedControlMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, control, data, message string
		copies                       int
	}{
		{name: "known destination retarget", control: "bind.mounts", data: `[{"source":"/source/resolv.conf","destination":"/etc/resolv.conf","file_type":"file","permissions":384}]`, copies: 1},
		{name: "unknown destination", control: "bind.mounts", data: `[{"source":"HOST_SENTINEL","destination":"/not-configured","file_type":"file","permissions":384}]`, copies: 1, message: "not configured"},
		{name: "duplicate destination", control: "bind.mounts", data: `[{"source":"/source/a","destination":"/etc/resolv.conf","file_type":"file"},{"source":"/source/b","destination":"/etc/resolv.conf","file_type":"file"}]`, copies: 1, message: "duplicate"},
		{name: "duplicate bind file", control: "bind.mounts", data: `[]`, copies: 2, message: "duplicate"},
		{name: "malformed bind file", control: "bind.mounts", data: `{`, copies: 1, message: "decode"},
		{name: "oversized bind file", control: "bind.mounts", data: strings.Repeat(" ", checkpointMetadataLimit+1), copies: 1, message: "invalid"},
		{name: "deletion traversal", control: metadata.DeletedFilesFile, data: `["../sentinel"]`, copies: 1, message: "unsafe"},
		{name: "deletion root", control: metadata.DeletedFilesFile, data: `["/"]`, copies: 1, message: "unsafe"},
		{name: "container root deletion path", control: metadata.DeletedFilesFile, data: `["/var/old"]`, copies: 1},
		{name: "duplicate deletion file", control: metadata.DeletedFilesFile, data: `[]`, copies: 2, message: "duplicate"},
		{name: "oversized deletion file", control: metadata.DeletedFilesFile, data: strings.Repeat(" ", checkpointMetadataLimit+1), copies: 1, message: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outside := filepath.Join(t.TempDir(), "uncreated")
			data := bytes.ReplaceAll([]byte(tc.data), []byte("HOST_SENTINEL"), []byte(outside))
			spec := &rspec.Spec{
				Process: &rspec.Process{},
				Linux:   &rspec.Linux{},
				Mounts: []rspec.Mount{
					{
						Type:        "bind",
						Source:      "/destination/resolv.conf",
						Destination: "/etc/resolv.conf",
					},
				},
			}
			source := makePodRestoreSecurityArchive(t, spec, tc.control, data, tc.copies)
			directory := t.TempDir()

			err := stagePodRestoreSecurityArchive(
				t,
				source,
				filepath.Join(directory, "staged.tar"),
				spec,
			)
			if tc.message == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.message)
				entries, err := os.ReadDir(directory)
				require.NoError(t, err)
				require.Empty(t, entries)
			}

			_, statErr := os.Stat(outside)
			require.ErrorIs(
				t,
				statErr,
				os.ErrNotExist,
				"staging must reject metadata before the host-path consumer",
			)
		})
	}
}

func stagePodRestoreSecurityArchive(
	t *testing.T,
	source, destination string,
	spec *rspec.Spec,
) error {
	t.Helper()

	return copyCheckpointArchive(source, destination, spec)
}

func TestPodRestoreResolvedResourceLimits(t *testing.T) {
	captured := &rspec.Spec{Linux: &rspec.Linux{}, Process: &rspec.Process{
		Rlimits: []rspec.POSIXRlimit{
			{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 1024},
			{Type: "RLIMIT_NPROC", Hard: 512, Soft: 256},
		},
	}}

	source := makePodRestoreSecurityArchive(t, captured, "", nil, 0)
	for _, kind := range []string{"unchanged", "reordered", "tightened hard", "tightened soft", "removed", "added"} {
		t.Run(kind, func(t *testing.T) {
			data, err := json.Marshal(captured)
			require.NoError(t, err)

			var destination rspec.Spec
			require.NoError(t, json.Unmarshal(data, &destination))

			switch kind {
			case "reordered":
				destination.Process.Rlimits[0], destination.Process.Rlimits[1] = destination.Process.Rlimits[1], destination.Process.Rlimits[0]
			case "tightened hard":
				destination.Process.Rlimits[1].Hard = 256
			case "tightened soft":
				destination.Process.Rlimits[0].Soft = 512
			case "removed":
				destination.Process.Rlimits = destination.Process.Rlimits[:1]
			case "added":
				destination.Process.Rlimits = append(
					destination.Process.Rlimits,
					rspec.POSIXRlimit{Type: "RLIMIT_MEMLOCK", Hard: 0, Soft: 0},
				)
			}

			before, err := json.Marshal(destination)
			require.NoError(t, err)
			directory := t.TempDir()

			err = stagePodRestoreSecurityArchive(
				t,
				source,
				filepath.Join(directory, "staged.tar"),
				&destination,
			)
			if kind == "unchanged" || kind == "reordered" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "resource limits")
				entries, err := os.ReadDir(directory)
				require.NoError(t, err)
				require.Empty(t, entries)
			}

			after, err := json.Marshal(destination)
			require.NoError(t, err)
			require.Equal(t, before, after, "normalization must not mutate the destination spec")
		})
	}

	require.Equal(t, "RLIMIT_NOFILE", captured.Process.Rlimits[0].Type)
}

func TestPodRestoreRejectsDuplicateResourceLimitTypes(t *testing.T) {
	for _, reversed := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "identical order", true: "reversed order"}[reversed],
			func(t *testing.T) {
				captured := &rspec.Spec{Linux: &rspec.Linux{}, Process: &rspec.Process{
					Rlimits: []rspec.POSIXRlimit{
						{Type: "RLIMIT_NOFILE", Hard: 1024, Soft: 128},
						{Type: "RLIMIT_NOFILE", Hard: 512, Soft: 256},
					},
				}}
				source := makePodRestoreSecurityArchive(t, captured, "", nil, 0)
				data, err := json.Marshal(captured)
				require.NoError(t, err)

				var destination rspec.Spec
				require.NoError(t, json.Unmarshal(data, &destination))

				if reversed {
					destination.Process.Rlimits[0], destination.Process.Rlimits[1] = destination.Process.Rlimits[1], destination.Process.Rlimits[0]
				}

				directory := t.TempDir()
				err = stagePodRestoreSecurityArchive(
					t,
					source,
					filepath.Join(directory, "staged.tar"),
					&destination,
				)
				require.ErrorContains(t, err, "resource limits")
				entries, err := os.ReadDir(directory)
				require.NoError(t, err)
				require.Empty(t, entries)
			},
		)
	}
}

func TestPodRestoreResolvedDevices(t *testing.T) {
	mode := os.FileMode(0o660)
	uid, gid := uint32(1000), uint32(1000)
	major, minor := int64(1), int64(3)
	captured := &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{
		Devices: []rspec.LinuxDevice{
			{
				Path:     "/dev/checkpoint",
				Type:     "c",
				Major:    major,
				Minor:    minor,
				FileMode: &mode,
				UID:      &uid,
				GID:      &gid,
			},
		},
		Resources: &rspec.LinuxResources{Devices: []rspec.LinuxDeviceCgroup{
			{Allow: false, Type: "a", Access: "rwm"},
			{Allow: true, Type: "c", Major: &major, Minor: &minor, Access: "rw"},
		}},
	}}

	source := makePodRestoreSecurityArchive(t, captured, "", nil, 0)
	for _, kind := range []string{
		"unchanged", "CPU allocation changed", "device path", "device type", "device major", "device minor",
		"device mode", "device UID", "device GID", "removed device", "added device",
		"ACL allow", "ACL type", "ACL major", "ACL minor", "ACL access", "removed ACL", "added ACL", "reordered ACL",
	} {
		t.Run(kind, func(t *testing.T) {
			data, err := json.Marshal(captured)
			require.NoError(t, err)

			var destination rspec.Spec
			require.NoError(t, json.Unmarshal(data, &destination))

			switch kind {
			case "CPU allocation changed":
				destination.Linux.Resources.CPU = &rspec.LinuxCPU{Cpus: "1"}
			case "device path":
				destination.Linux.Devices[0].Path = "/dev/other"
			case "device type":
				destination.Linux.Devices[0].Type = "b"
			case "device major":
				destination.Linux.Devices[0].Major++
			case "device minor":
				destination.Linux.Devices[0].Minor++
			case "device mode":
				*destination.Linux.Devices[0].FileMode = 0o600
			case "device UID":
				*destination.Linux.Devices[0].UID = 2000
			case "device GID":
				*destination.Linux.Devices[0].GID = 2000
			case "removed device":
				destination.Linux.Devices = nil
			case "added device":
				destination.Linux.Devices = append(
					destination.Linux.Devices,
					rspec.LinuxDevice{Path: "/dev/other", Type: "c", Major: 1, Minor: 5},
				)
			case "ACL allow":
				destination.Linux.Resources.Devices[1].Allow = false
			case "ACL type":
				destination.Linux.Resources.Devices[1].Type = "b"
			case "ACL major":
				*destination.Linux.Resources.Devices[1].Major++
			case "ACL minor":
				*destination.Linux.Resources.Devices[1].Minor++
			case "ACL access":
				destination.Linux.Resources.Devices[1].Access = "r"
			case "removed ACL":
				destination.Linux.Resources.Devices = destination.Linux.Resources.Devices[:1]
			case "added ACL":
				destination.Linux.Resources.Devices = append(
					destination.Linux.Resources.Devices,
					rspec.LinuxDeviceCgroup{Allow: false, Type: "b", Access: "rwm"},
				)
			case "reordered ACL":
				destination.Linux.Resources.Devices[0], destination.Linux.Resources.Devices[1] = destination.Linux.Resources.Devices[1], destination.Linux.Resources.Devices[0]
			}

			before, err := json.Marshal(destination)
			require.NoError(t, err)
			directory := t.TempDir()

			err = stagePodRestoreSecurityArchive(
				t,
				source,
				filepath.Join(directory, "staged.tar"),
				&destination,
			)
			if kind == "unchanged" || kind == "CPU allocation changed" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "resolved devices")
				entries, err := os.ReadDir(directory)
				require.NoError(t, err)
				require.Empty(t, entries)
			}

			after, err := json.Marshal(destination)
			require.NoError(t, err)
			require.Equal(
				t,
				before,
				after,
				"device comparison must preserve ACL order and the destination spec",
			)
		})
	}

	require.Equal(t, int64(1), *captured.Linux.Resources.Devices[1].Major)
}

func TestPodRestoreEmptyDeviceRules(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "nil source resources", true: "nil destination resources"}[reverse],
			func(t *testing.T) {
				captured := &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{}}

				destination := &rspec.Spec{Process: &rspec.Process{}, Linux: &rspec.Linux{
					Devices: []rspec.LinuxDevice{},
					Resources: &rspec.LinuxResources{
						Devices: []rspec.LinuxDeviceCgroup{},
						CPU:     &rspec.LinuxCPU{Cpus: "1"},
					},
				}}
				if reverse {
					captured, destination = destination, captured
				}

				source := makePodRestoreSecurityArchive(t, captured, "", nil, 0)
				require.NoError(
					t,
					stagePodRestoreSecurityArchive(
						t,
						source,
						filepath.Join(t.TempDir(), "staged.tar"),
						destination,
					),
				)
			},
		)
	}
}
