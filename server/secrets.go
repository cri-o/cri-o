package server

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/selinux/go-selinux/label"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// SecretData info
type SecretData struct {
	Name string
	Data []byte
}

// SaveTo saves secret data to given directory
func (s SecretData) SaveTo(dir string) error {
	path := filepath.Join(dir, s.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil && !os.IsExist(err) {
		return err
	}
	return ioutil.WriteFile(path, s.Data, 0700)
}

// readMountFile returns a list of the host:container paths
func readMountFile(mountFilePath string) ([]string, error) {
	var mountPaths []string
	file, err := os.Open(mountFilePath)
	if err != nil {
		logrus.Debugf("file doesn't exist %q, skipping...", mountFilePath)
		return nil, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		mountPaths = append(mountPaths, scanner.Text())
	}

	return mountPaths, nil
}

func readAll(root, prefix string) ([]SecretData, error) {
	path := filepath.Join(root, prefix)

	data := []SecretData{}

	files, err := ioutil.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return data, nil
		}

		return nil, err
	}

	for _, f := range files {
		fileData, err := readFile(root, filepath.Join(prefix, f.Name()))
		if err != nil {
			// If the file did not exist, might be a dangling symlink
			// Ignore the error
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		data = append(data, fileData...)
	}

	return data, nil
}

func readFile(root, name string) ([]SecretData, error) {
	path := filepath.Join(root, name)

	s, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if s.IsDir() {
		dirData, err := readAll(root, name)
		if err != nil {
			return nil, err
		}
		return dirData, nil
	}
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return []SecretData{{Name: name, Data: bytes}}, nil
}

// getHostAndCtrDir separates the host:container paths
func getMountsMap(path string) (string, string, error) {
	arr := strings.SplitN(path, ":", 2)
	if len(arr) == 2 {
		return arr[0], arr[1], nil
	}
	return "", "", errors.Errorf("unable to get host and container dir")
}

func getHostSecretData(hostDir string) ([]SecretData, error) {
	var allSecrets []SecretData
	hostSecrets, err := readAll(hostDir, "")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read secrets from %q", hostDir)
	}
	return append(allSecrets, hostSecrets...), nil
}

// secretMount copies the contents of host directory to container directory
// and returns a list of mounts
func secretMounts(mountLabel, mountFilePath, containerWorkingDir string, runtimeMounts []rspec.Mount) ([]rspec.Mount, error) {
	var mounts []rspec.Mount
	mountPaths, err := readMountFile(mountFilePath)
	if err != nil {
		return nil, err
	}
	for _, path := range mountPaths {
		hostDir, ctrDir, err := getMountsMap(path)
		if err != nil {
			return nil, err
		}
		// skip if the hostDir path doesn't exist
		if _, err := os.Stat(hostDir); os.IsNotExist(err) {
			logrus.Warnf("%q doesn't exist, skipping", hostDir)
			continue
		}

		ctrDir = filepath.Join(containerWorkingDir, ctrDir)
		// skip if ctrDir has already been mounted by caller
		if isAlreadyMounted(runtimeMounts, ctrDir) {
			logrus.Warnf("%q has already been mounted; cannot override mount", ctrDir)
			continue
		}

		if err := os.RemoveAll(ctrDir); err != nil {
			return nil, fmt.Errorf("remove container directory failed: %v", err)
		}

		if err := os.MkdirAll(ctrDir, 0755); err != nil {
			return nil, fmt.Errorf("making container directory failed: %v", err)
		}

		hostDir, err = resolveSymbolicLink(hostDir)
		if err != nil {
			return nil, err
		}

		data, err := getHostSecretData(hostDir)
		if err != nil {
			return nil, errors.Wrapf(err, "getting host secret data failed")
		}
		for _, s := range data {
			s.SaveTo(ctrDir)
		}
		label.Relabel(ctrDir, mountLabel, false)

		m := rspec.Mount{
			Source:      hostDir,
			Destination: ctrDir,
		}

		mounts = append(mounts, m)
	}
	return mounts, nil
}

func isAlreadyMounted(mounts []rspec.Mount, mountPath string) bool {
	for _, mount := range mounts {
		if mount.Destination == mountPath {
			return true
		}
	}
	return false
}
