package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	criPodSandboxConfigFile = "pod-sandbox-config.json"
	criContainerConfigFile  = "container-config.json"
	checkpointMetadataLimit = 4 << 20
)

func persistPodSandboxConfig(dir string, config *types.PodSandboxConfig) error {
	path := filepath.Join(dir, criPodSandboxConfigFile)
	if _, err := os.Lstat(path); err == nil {
		existing, err := loadPodSandboxConfig(dir)
		if err != nil {
			return err
		}

		if !proto.Equal(existing, config) {
			return errors.New("persisted pod sandbox checkpoint metadata does not match the creation request")
		}

		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	return writeCheckpointJSON(dir, criPodSandboxConfigFile, config)
}

func persistContainerConfig(dir string, config *types.ContainerConfig) error {
	path := filepath.Join(dir, criContainerConfigFile)
	if _, err := os.Lstat(path); err == nil {
		existing, err := loadContainerConfig(dir)
		if err != nil {
			return err
		}

		if !proto.Equal(existing, config) {
			return errors.New("persisted container checkpoint metadata does not match the creation request")
		}

		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	return writeCheckpointJSON(dir, criContainerConfigFile, config)
}

func loadPodSandboxConfig(dir string) (*types.PodSandboxConfig, error) {
	config := new(types.PodSandboxConfig)
	if err := readCheckpointJSON(filepath.Join(dir, criPodSandboxConfigFile), config); err != nil {
		return nil, err
	}

	return config, nil
}

func loadContainerConfig(dir string) (*types.ContainerConfig, error) {
	config := new(types.ContainerConfig)
	if err := readCheckpointJSON(filepath.Join(dir, criContainerConfigFile), config); err != nil {
		return nil, err
	}

	return config, nil
}

func writeCheckpointJSON(dir, name string, value any) (retErr error) {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}

	temporary, err := os.CreateTemp(dir, "."+name+"-")
	if err != nil {
		return fmt.Errorf("create temporary %s: %w", name, err)
	}

	temporaryName := temporary.Name()

	closed := false
	defer func() {
		if !closed {
			if closeErr := temporary.Close(); closeErr != nil && retErr == nil {
				retErr = fmt.Errorf("close temporary %s: %w", name, closeErr)
			}
		}

		if retErr != nil {
			_ = os.Remove(temporaryName)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("set permissions on temporary %s: %w", name, err)
	}

	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write temporary %s: %w", name, err)
	}

	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary %s: %w", name, err)
	}

	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary %s: %w", name, err)
	}

	closed = true

	if err := os.Rename(temporaryName, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("publish %s: %w", name, err)
	}

	return syncCheckpointDirectory(dir)
}

func readCheckpointJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", path, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}

	if info.Size() > checkpointMetadataLimit {
		return fmt.Errorf("%s exceeds the %d-byte metadata limit", path, checkpointMetadataLimit)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode %s: multiple JSON values", path)
		}

		return fmt.Errorf("decode %s: %w", path, err)
	}

	return nil
}

func syncCheckpointDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory %s for sync: %w", path, err)
	}
	defer directory.Close()

	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory %s: %w", path, err)
	}

	return nil
}
