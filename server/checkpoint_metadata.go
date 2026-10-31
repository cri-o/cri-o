package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
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
			return errors.New(
				"persisted pod sandbox checkpoint metadata does not match the creation request",
			)
		}

		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
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
			return errors.New(
				"persisted container checkpoint metadata does not match the creation request",
			)
		}

		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
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

	if len(data) > checkpointMetadataLimit {
		return fmt.Errorf("%s exceeds checkpoint metadata limit", name)
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

// Open first, without following links or blocking on FIFOs, then validate
// the descriptor. Checking a path before opening it can inspect another file.
func openCheckpointFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	return regularCheckpointFile(os.NewFile(uintptr(fd), path))
}

func regularCheckpointFile(file *os.File) (*os.File, error) {
	info, err := file.Stat()
	if err != nil {
		file.Close()

		return nil, err
	}

	if !info.Mode().IsRegular() {
		file.Close()

		return nil, fmt.Errorf("%s is not a regular file", file.Name())
	}

	return file, nil
}

func openCheckpointDirectory(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(fd), path), nil
}

func openCheckpointFileAt(directory *os.File, name string) (*os.File, error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return nil, fmt.Errorf("invalid checkpoint filename %q", name)
	}

	fd, err := unix.Openat(
		int(directory.Fd()),
		name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, err
	}

	return regularCheckpointFile(os.NewFile(uintptr(fd), filepath.Join(directory.Name(), name)))
}

func openCheckpointDirectoryAt(directory *os.File, name string) (*os.File, error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return nil, fmt.Errorf("invalid checkpoint directory name %q", name)
	}

	fd, err := unix.Openat(
		int(directory.Fd()),
		name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY,
		0,
	)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(fd), filepath.Join(directory.Name(), name)), nil
}

func readCheckpointJSON(path string, value any) error {
	file, err := openCheckpointFile(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	return readCheckpointJSONFile(file, value)
}

func readCheckpointJSONAt(directory *os.File, name string, value any) error {
	file, err := openCheckpointFileAt(directory, name)
	if err != nil {
		return fmt.Errorf("open checkpoint metadata %s: %w", name, err)
	}
	defer file.Close()

	return readCheckpointJSONFile(file, value)
}

func readCheckpointJSONFile(file *os.File, value any) error {
	data, err := io.ReadAll(io.LimitReader(file, checkpointMetadataLimit+1))
	if err != nil {
		return fmt.Errorf("read %s: %w", file.Name(), err)
	}

	if len(data) > checkpointMetadataLimit {
		return fmt.Errorf(
			"%s exceeds the %d-byte metadata limit",
			file.Name(),
			checkpointMetadataLimit,
		)
	}

	return decodeCheckpointJSON(data, value)
}

func decodeCheckpointJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode checkpoint metadata: %w", err)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("checkpoint metadata contains multiple JSON values")
		}

		return fmt.Errorf("decode checkpoint metadata: %w", err)
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
