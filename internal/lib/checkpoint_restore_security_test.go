//go:build linux

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

package lib

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"
	"go.podman.io/storage/pkg/reexec"
	"golang.org/x/sys/unix"
)

func TestMain(m *testing.M) {
	if reexec.Init() {
		return
	}

	os.Exit(m.Run())
}

func TestCheckpointDeletedFilesConfinement(t *testing.T) {
	for _, unsafe := range []string{"../outside", "/../outside", "a/../../outside", "/", ".", "./", "././", ""} {
		t.Run(unsafe, func(t *testing.T) {
			directory, rootfs := t.TempDir(), t.TempDir()
			own := filepath.Join(rootfs, "own")
			require.NoError(t, os.WriteFile(own, []byte("keep"), 0o600))

			data, err := json.Marshal([]string{"own", unsafe})
			require.NoError(t, err)
			require.NoError(
				t,
				os.WriteFile(filepath.Join(directory, metadata.DeletedFilesFile), data, 0o600),
			)
			require.Error(t, removeCheckpointDeletedFiles(directory, rootfs))
			require.FileExists(t, own, "validate every path before deleting any file")
		})
	}

	t.Run("ancestor symlink", func(t *testing.T) {
		directory, rootfs, outside := t.TempDir(), t.TempDir(), t.TempDir()
		sentinel := filepath.Join(outside, "sentinel")
		require.NoError(t, os.WriteFile(sentinel, []byte("keep"), 0o600))
		require.NoError(t, os.Symlink(outside, filepath.Join(rootfs, "link")))
		require.NoError(
			t,
			os.WriteFile(
				filepath.Join(directory, metadata.DeletedFilesFile),
				[]byte(`["link/sentinel"]`),
				0o600,
			),
		)
		require.Error(t, removeCheckpointDeletedFiles(directory, rootfs))

		value, err := os.ReadFile(sentinel)
		require.NoError(t, err)
		require.Equal(t, "keep", string(value))
	})
	t.Run("normal root relative paths", func(t *testing.T) {
		directory, rootfs := t.TempDir(), t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(rootfs, "nested"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(rootfs, "nested", "old"), nil, 0o600))
		require.NoError(
			t,
			os.WriteFile(
				filepath.Join(directory, metadata.DeletedFilesFile),
				[]byte(`["/nested/old","nested"]`),
				0o600,
			),
		)
		require.NoError(t, removeCheckpointDeletedFiles(directory, rootfs))
		require.NoDirExists(t, filepath.Join(rootfs, "nested"))
	})
	t.Run("oversized control file", func(t *testing.T) {
		directory := t.TempDir()
		require.NoError(
			t,
			os.WriteFile(
				filepath.Join(directory, metadata.DeletedFilesFile),
				bytes.Repeat([]byte(" "), checkpointControlMetadataLimit+1),
				0o600,
			),
		)
		require.Error(t, removeCheckpointDeletedFiles(directory, t.TempDir()))
	})
}

func TestCheckpointRootfsDiffConfinement(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root in a disposable user namespace for chroot extraction")
	}

	for _, escape := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "normal", true: "preexisting ancestor symlink"}[escape],
			func(t *testing.T) {
				directory, rootfs, outside := t.TempDir(), t.TempDir(), t.TempDir()
				sentinel := filepath.Join(outside, "sentinel")
				require.NoError(t, os.WriteFile(sentinel, []byte("keep"), 0o600))

				if escape {
					require.NoError(t, os.Symlink(outside, filepath.Join(rootfs, "nested")))
				}

				file, err := os.Create(filepath.Join(directory, metadata.RootFsDiffTar))
				require.NoError(t, err)

				writer := tar.NewWriter(file)
				require.NoError(
					t,
					writer.WriteHeader(
						&tar.Header{
							Name:     "nested/sentinel",
							Typeflag: tar.TypeReg,
							Mode:     0o600,
							Size:     7,
						},
					),
				)
				_, err = writer.Write([]byte("changed"))
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				require.NoError(t, file.Close())

				err = applyCheckpointRootfsDiff(directory, rootfs)
				if !escape {
					require.NoError(t, err)
					data, err := os.ReadFile(filepath.Join(rootfs, "nested", "sentinel"))
					require.NoError(t, err)
					require.Equal(t, "changed", string(data))
				}

				data, readErr := os.ReadFile(sentinel)
				require.NoError(t, readErr)
				require.Equal(t, "keep", string(data))
			},
		)
	}
}

func TestCheckpointBindMountConfinement(t *testing.T) {
	for _, kind := range []string{"unknown destination", "known retarget", "missing configured source"} {
		t.Run(kind, func(t *testing.T) {
			directory, outside := t.TempDir(), t.TempDir()
			source := filepath.Join(outside, "sentinel")
			destination := &rspec.Spec{
				Mounts: []rspec.Mount{
					{Type: "bind", Destination: "/etc/resolv.conf", Source: source},
				},
			}
			mount := ExternalBindMount{
				Source:      source,
				Destination: "/etc/resolv.conf",
				FileType:    "file",
				Permissions: 0o600,
			}

			switch kind {
			case "unknown destination":
				mount.Destination = "/unconfigured"
			case "known retarget":
				destination.Mounts[0].Source = filepath.Join(outside, "retarget")
			}

			data, err := json.Marshal([]ExternalBindMount{mount})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "bind.mounts"), data, 0o600))

			err = restoreCheckpointBindMounts(directory, destination)
			if kind == "unknown destination" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if kind == "missing configured source" {
				require.FileExists(t, source)
			} else {
				require.NoFileExists(t, source)
			}
		})
	}
}

func TestCheckpointRejectsAmbiguousBindSources(t *testing.T) {
	mounts := []ExternalBindMount{
		{Source: "/captured/source", Destination: "/first", FileType: "file", Permissions: 0o600},
		{
			Source:      "/captured/source",
			Destination: "/second",
			FileType:    "directory",
			Permissions: 0o700,
		},
	}
	destination := &rspec.Spec{Mounts: []rspec.Mount{
		{Type: "bind", Destination: "/first", Source: "/current/first"},
		{Type: "bind", Destination: "/second", Source: "/current/second"},
	}}
	require.ErrorContains(t, ValidateCheckpointBindMounts(mounts, destination), "ambiguous")
	mounts[1].FileType, mounts[1].Permissions = mounts[0].FileType, mounts[0].Permissions
	require.NoError(t, ValidateCheckpointBindMounts(mounts, destination))
}

func TestCheckpointControlFileTypes(t *testing.T) {
	for _, kind := range []string{"regular", "symlink", "fifo", "directory"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			name := filepath.Join(directory, "control")

			switch kind {
			case "regular":
				require.NoError(t, os.WriteFile(name, []byte("safe"), 0o600))
			case "symlink":
				require.NoError(
					t,
					os.WriteFile(filepath.Join(directory, "target"), []byte("safe"), 0o600),
				)
				require.NoError(t, os.Symlink("target", name))
			case "fifo":
				require.NoError(t, unix.Mkfifo(name, 0o600))
			case "directory":
				require.NoError(t, os.Mkdir(name, 0o700))
			}

			result := make(chan error, 1)

			go func() {
				file, err := openCheckpointControlFile(directory, "control")
				if file != nil {
					file.Close()
				}

				result <- err
			}()

			select {
			case err := <-result:
				if kind == "regular" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			case <-time.After(time.Second):
				t.Fatal("opening checkpoint control file blocked")
			}
		})
	}
}
