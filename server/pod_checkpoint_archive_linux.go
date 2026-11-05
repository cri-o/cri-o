//go:build linux

package server

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"go.podman.io/storage/pkg/archive"

	"github.com/cri-o/cri-o/internal/storage"
)

// Older Pod manifests did not record ImageID. Their verified container
// archives already contain the immutable storage image ID in config.dump.
func checkpointArchiveImageID(
	file *os.File,
	container *podCheckpointManifestContainer,
	runtimeHandler string,
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

	var config *metadata.ContainerConfig

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return "", fmt.Errorf("read checkpoint archive: %w", err)
		}

		if path.Clean(header.Name) != metadata.ConfigDumpFile {
			continue
		}

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

	return id, nil
}
