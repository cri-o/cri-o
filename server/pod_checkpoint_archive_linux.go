//go:build linux

package server

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/checkpoint-restore/go-criu/v8/stats"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"go.podman.io/storage/pkg/archive"

	"github.com/cri-o/cri-o/internal/annotations"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/storage"
)

// Older Pod manifests did not record ImageID. Their verified container
// archives already contain the immutable storage image ID in config.dump.
func checkpointArchiveImageID(
	file *os.File,
	container *podCheckpointManifestContainer,
	runtimeHandler string,
	destination *rspec.Spec,
) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	stream, err := archive.DecompressStream(io.NewSectionReader(file, 0, info.Size()))
	if err != nil {
		return "", err
	}
	defer stream.Close()

	reader := tar.NewReader(stream)

	var (
		config         *metadata.ContainerConfig
		capturedSpec   *rspec.Spec
		bindMounts     []lib.ExternalBindMount
		bindsFound     bool
		deletionsFound bool
	)

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", fmt.Errorf("read checkpoint archive: %w", err)
		}

		name := path.Clean(header.Name)
		if err := validateCheckpointArchiveEntry(name, header.Typeflag); err != nil {
			return "", err
		}

		switch {
		case name == metadata.ConfigDumpFile:
			if config != nil || header.Typeflag != tar.TypeReg ||
				header.Size > checkpointMetadataLimit {
				return "", errors.New("checkpoint archive has invalid or duplicate config metadata")
			}

			data, err := io.ReadAll(io.LimitReader(reader, checkpointMetadataLimit+1))
			if err != nil {
				return "", err
			}

			config = new(metadata.ContainerConfig)
			if err := json.Unmarshal(data, config); err != nil {
				return "", fmt.Errorf("decode archive config: %w", err)
			}
		case name == metadata.SpecDumpFile && destination != nil:
			if capturedSpec != nil || header.Typeflag != tar.TypeReg ||
				header.Size > checkpointMetadataLimit {
				return "", errors.New(
					"checkpoint archive has invalid or duplicate OCI spec metadata",
				)
			}

			data, err := io.ReadAll(io.LimitReader(reader, checkpointMetadataLimit+1))
			if err != nil {
				return "", err
			}

			capturedSpec = new(rspec.Spec)
			if err := json.Unmarshal(data, capturedSpec); err != nil {
				return "", fmt.Errorf("decode checkpoint OCI spec: %w", err)
			}
		case name == metadata.DeletedFilesFile && destination != nil:
			if deletionsFound || header.Typeflag != tar.TypeReg ||
				header.Size > checkpointMetadataLimit {
				return "", errors.New(
					"checkpoint archive has invalid or duplicate deletion metadata",
				)
			}

			deletionsFound = true

			data, err := io.ReadAll(io.LimitReader(reader, checkpointMetadataLimit+1))
			if err != nil {
				return "", err
			}

			if _, err := lib.ValidateCheckpointDeletedFiles(data); err != nil {
				return "", err
			}
		case name == "bind.mounts" && destination != nil:
			if bindsFound || header.Typeflag != tar.TypeReg ||
				header.Size > checkpointMetadataLimit {
				return "", errors.New(
					"checkpoint archive has invalid or duplicate bind mount metadata",
				)
			}

			bindsFound = true

			data, err := io.ReadAll(io.LimitReader(reader, checkpointMetadataLimit+1))
			if err != nil {
				return "", err
			}

			if err := json.Unmarshal(data, &bindMounts); err != nil {
				return "", fmt.Errorf("decode checkpoint bind mounts: %w", err)
			}
		}
	}

	if config == nil || config.ID != container.ID || config.OCIRuntime != runtimeHandler {
		return "", errors.New("checkpoint archive identity does not match the Pod manifest")
	}

	imageID, err := storage.ParseStorageImageIDFromOutOfProcessData(config.RootfsImageRef)
	if err != nil {
		return "", fmt.Errorf("checkpoint archive has no valid immutable base image ID: %w", err)
	}

	id := imageID.IDStringForOutOfProcessConsumptionOnly()
	if container.ImageID != "" && container.ImageID != id {
		return "", errors.New("checkpoint archive base image does not match the Pod manifest")
	}

	if destination != nil {
		if err := validateCheckpointSecurity(capturedSpec, destination); err != nil {
			return "", err
		}

		if err := lib.ValidateCheckpointBindMounts(bindMounts, destination); err != nil {
			return "", err
		}
	}
	return id, nil
}

// checkpointArchiveFiles lists the regular files that a container checkpoint
// archive can contain outside the CRIU image directory.
var checkpointArchiveFiles = map[string]bool{
	metadata.ConfigDumpFile:   true,
	metadata.SpecDumpFile:     true,
	metadata.DumpLogFile:      true,
	stats.StatsDump:           true,
	"bind.mounts":             true,
	metadata.RootFsDiffTar:    true,
	metadata.DeletedFilesFile: true,
	annotations.LogPath:       true,
}

// validateCheckpointArchiveEntry rejects entries that restore would extract
// over CRI-O's own container state, such as state.json or config.json.
func validateCheckpointArchiveEntry(name string, typeflag byte) error {
	switch {
	case name == "." || name == metadata.CheckpointDirectory:
		if typeflag == tar.TypeDir {
			return nil
		}
	case checkpointArchiveFiles[name]:
		if typeflag == tar.TypeReg {
			return nil
		}
	case path.Dir(name) == metadata.CheckpointDirectory:
		if typeflag == tar.TypeReg {
			return nil
		}
	}

	return fmt.Errorf("checkpoint archive has unexpected entry %q", name)
}

// checkpointArchiveDigest hashes an exported archive before it is published.
// The archive writer skips files that it cannot read, so the same pass checks
// that the archive still contains the files that restore requires.
func checkpointArchiveDigest(archivePath string) (string, error) {
	file, err := openCheckpointFile(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	stream := io.TeeReader(file, hash)
	reader := tar.NewReader(stream)

	required := map[string]bool{
		metadata.ConfigDumpFile: false,
		metadata.SpecDumpFile:   false,
		path.Join(metadata.CheckpointDirectory, "inventory.img"): false,
	}

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", fmt.Errorf("read checkpoint archive: %w", err)
		}

		name := path.Clean(header.Name)
		if err := validateCheckpointArchiveEntry(name, header.Typeflag); err != nil {
			return "", err
		}

		if _, ok := required[name]; ok {
			required[name] = true
		}
	}

	for name, found := range required {
		if !found {
			return "", fmt.Errorf("checkpoint archive is missing %q", name)
		}
	}

	// Hash any padding after the end-of-archive marker, too.
	if _, err := io.Copy(io.Discard, stream); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
