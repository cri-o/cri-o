// SPDX-License-Identifier: Apache-2.0

package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	spec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
)

const (
	// container archive
	ConfigDumpFile             = "config.dump"
	SpecDumpFile               = "spec.dump"
	NetworkStatusFile          = "network.status"
	CheckpointDirectory        = "checkpoint"
	CheckpointVolumesDirectory = "volumes"
	DevShmCheckpointTar        = "devshm-checkpoint.tar"
	RootFsDiffTar              = "rootfs-diff.tar"
	DeletedFilesFile           = "deleted.files"
	DumpLogFile                = "dump.log"
	RestoreLogFile             = "restore.log"
	// pod archive
	PodOptionsFile = "pod.options"
	PodDumpFile    = "pod.dump"
	// containerd only
	StatusFile = "status"
)

// This is a reduced copy of what Podman uses to store checkpoint metadata
type ContainerConfig struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	RootfsImage     string    `json:"rootfsImage,omitempty"`
	RootfsImageRef  string    `json:"rootfsImageRef,omitempty"`
	RootfsImageName string    `json:"rootfsImageName,omitempty"`
	OCIRuntime      string    `json:"runtime,omitempty"`
	CreatedTime     time.Time `json:"createdTime"`
	CheckpointedAt  time.Time `json:"checkpointedTime"`
	RestoredAt      time.Time `json:"restoredTime"`
	Restored        bool      `json:"restored"`
}

type ContainerdStatus struct {
	CreatedAt  int64
	StartedAt  int64
	FinishedAt int64
	ExitCode   int32
	Pid        uint32
	Reason     string
	Message    string
}

func ReadContainerCheckpointSpecDump(checkpointDirectory string) (*spec.Spec, string, error) {
	var specDump spec.Spec
	specDumpFile, err := ReadJSONFile(&specDump, checkpointDirectory, SpecDumpFile)

	return &specDump, specDumpFile, err
}

func ReadContainerCheckpointConfigDump(checkpointDirectory string) (*ContainerConfig, string, error) {
	var containerConfig ContainerConfig
	configDumpFile, err := ReadJSONFile(&containerConfig, checkpointDirectory, ConfigDumpFile)

	return &containerConfig, configDumpFile, err
}

func ReadContainerCheckpointDeletedFiles(checkpointDirectory string) ([]string, string, error) {
	var deletedFiles []string
	deletedFilesFile, err := ReadJSONFile(&deletedFiles, checkpointDirectory, DeletedFilesFile)

	return deletedFiles, deletedFilesFile, err
}

func ReadContainerCheckpointStatusFile(checkpointDirectory string) (*ContainerdStatus, string, error) {
	var containerdStatus ContainerdStatus
	statusFile, err := ReadJSONFile(&containerdStatus, checkpointDirectory, StatusFile)

	return &containerdStatus, statusFile, err
}

// WriteJSONFile marshalls and writes the given data to a JSON file
func WriteJSONFile(v interface{}, dir, file string) (string, error) {
	fileJSON, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", errors.Wrapf(err, "Error marshalling JSON")
	}
	file = filepath.Join(dir, file)
	if err := os.WriteFile(file, fileJSON, 0o600); err != nil {
		return "", errors.Wrapf(err, "Error writing to %q", file)
	}

	return file, nil
}

func ReadJSONFile(v interface{}, dir, file string) (string, error) {
	file = filepath.Join(dir, file)
	content, err := os.ReadFile(file)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %s", file)
	}
	if err = json.Unmarshal(content, v); err != nil {
		return "", errors.Wrapf(err, "failed to unmarshal %s", file)
	}

	return file, nil
}

func ByteToString(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %ciB",
		float64(b)/float64(div), "KMGTPE"[exp])
}
