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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/storage/pkg/chrootarchive"
	"golang.org/x/sys/unix"
)

const checkpointControlMetadataLimit = 1 << 20

// ValidateCheckpointDeletedFiles accepts container-root paths emitted by storage
// diff, but never parent traversal or deletion of the root itself.
func ValidateCheckpointDeletedFiles(data []byte) ([]string, error) {
	if len(data) > checkpointControlMetadataLimit {
		return nil, errors.New("checkpoint deletion metadata exceeds the size limit")
	}

	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("decode checkpoint deletion metadata: %w", err)
	}

	for i, name := range names {
		relative := strings.TrimPrefix(name, "/")
		if !filepath.IsLocal(relative) || filepath.Clean(relative) == "." ||
			strings.Contains("/"+relative+"/", "/../") {
			return nil, errors.New("checkpoint deletion metadata contains an unsafe path")
		}

		names[i] = relative
	}

	return names, nil
}

func removeCheckpointDeletedFiles(directory, rootfs string) error {
	file, err := openCheckpointControlFile(directory, metadata.DeletedFilesFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() || info.Size() > checkpointControlMetadataLimit {
		return errors.New("checkpoint deletion metadata must be a bounded regular file")
	}

	data, err := io.ReadAll(io.LimitReader(file, checkpointControlMetadataLimit+1))
	if err != nil {
		return err
	}

	names, err := ValidateCheckpointDeletedFiles(data)
	if err != nil {
		return err
	}

	root, err := os.OpenRoot(rootfs)
	if err != nil {
		return err
	}
	defer root.Close()

	for _, name := range names {
		if err := root.RemoveAll(name); err != nil {
			return fmt.Errorf("remove checkpointed rootfs path: %w", err)
		}
	}

	return nil
}

func applyCheckpointRootfsDiff(directory, rootfs string) error {
	file, err := openCheckpointControlFile(directory, metadata.RootFsDiffTar)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() {
		return errors.New("checkpoint rootfs diff must be a regular file")
	}
	// A rootfs may already contain ancestor symlinks before extraction begins.
	// Confinement must hold at the consumer, not just for tar entry names.
	if err := chrootarchive.Untar(file, rootfs, nil); err != nil {
		return fmt.Errorf("apply checkpoint rootfs diff: %w", err)
	}

	return nil
}

func openCheckpointControlFile(directory, name string) (*os.File, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	if name == "." || name == ".." || name == "" || filepath.Base(name) != name {
		return nil, errors.New("checkpoint control file name must be a base name")
	}

	directoryFile, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directoryFile.Close()
	// os.Root resolves in-root symlinks even when OpenFile receives O_NOFOLLOW.
	// Open a single component relative to the held directory instead.
	fd, err := unix.Openat(
		int(directoryFile.Fd()),
		name,
		unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open checkpoint control file: %w", err)
	}

	file := os.NewFile(uintptr(fd), name)

	info, err := file.Stat()
	if err != nil {
		file.Close()

		return nil, err
	}

	if !info.Mode().IsRegular() {
		file.Close()

		return nil, errors.New("checkpoint control file must be a regular file")
	}

	return file, nil
}

// ValidateCheckpointBindMounts permits captured host paths only when their
// destinations belong to the destination container's bind mounts.
func ValidateCheckpointBindMounts(mounts []ExternalBindMount, destination *rspec.Spec) error {
	if destination == nil {
		return errors.New("restore container has no resolved mount configuration")
	}

	configured := make(map[string]rspec.Mount, len(destination.Mounts))
	for _, mount := range destination.Mounts {
		if _, exists := configured[mount.Destination]; exists {
			return errors.New("restore container has ambiguous mount destinations")
		}

		configured[mount.Destination] = mount
	}

	seen := make(map[string]bool, len(mounts))

	sources := make(map[string]ExternalBindMount, len(mounts))
	for _, mount := range mounts {
		if seen[mount.Destination] || !path.IsAbs(mount.Destination) ||
			path.Clean(mount.Destination) != mount.Destination ||
			!path.IsAbs(mount.Source) ||
			path.Clean(mount.Source) != mount.Source ||
			(mount.FileType != "file" && mount.FileType != "directory") ||
			mount.Permissions&^uint32(0o777) != 0 {
			return errors.New("checkpoint archive has invalid or duplicate external bind mounts")
		}

		seen[mount.Destination] = true
		if earlier, exists := sources[mount.Source]; exists &&
			(earlier.FileType != mount.FileType || earlier.Permissions != mount.Permissions) {
			return errors.New("checkpoint archive has ambiguous external bind mount sources")
		}

		sources[mount.Source] = mount

		target, exists := configured[mount.Destination]
		if !exists || target.Type != "bind" {
			return errors.New(
				"checkpoint external bind mount destination is not configured for restore",
			)
		}
	}

	return nil
}

func restoreCheckpointBindMounts(directory string, destination *rspec.Spec) error {
	file, err := openCheckpointControlFile(directory, "bind.mounts")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() || info.Size() > checkpointControlMetadataLimit {
		return errors.New("checkpoint bind mount metadata must be a bounded regular file")
	}

	data, err := io.ReadAll(io.LimitReader(file, checkpointControlMetadataLimit+1))
	if err != nil {
		return err
	}

	if len(data) > checkpointControlMetadataLimit {
		return errors.New("checkpoint bind mount metadata exceeds the size limit")
	}

	var mounts []ExternalBindMount
	if err := json.Unmarshal(data, &mounts); err != nil {
		return fmt.Errorf("decode checkpoint bind mounts: %w", err)
	}

	if err := ValidateCheckpointBindMounts(mounts, destination); err != nil {
		return err
	}

	configured := make(map[string]string, len(destination.Mounts))
	for _, mount := range destination.Mounts {
		configured[mount.Destination] = mount.Source
	}

	for _, mount := range mounts {
		// Destination configuration takes precedence over captured host paths.
		if configured[mount.Destination] != mount.Source {
			continue
		}

		if _, err := os.Lstat(mount.Source); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		if mount.FileType == "directory" {
			if err := os.MkdirAll(mount.Source, os.FileMode(mount.Permissions)); err != nil {
				return fmt.Errorf(
					"recreate checkpoint bind mount directory %q: %w",
					mount.Source,
					err,
				)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(mount.Source), 0o700); err != nil {
				return err
			}

			source, err := os.OpenFile(
				mount.Source,
				os.O_RDONLY|os.O_CREATE,
				os.FileMode(mount.Permissions),
			)
			if err != nil {
				return fmt.Errorf("recreate checkpoint bind mount file %q: %w", mount.Source, err)
			}

			if err := source.Close(); err != nil {
				return err
			}
		}
	}

	return nil
}
