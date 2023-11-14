/*
Copyright 2022 The Kubernetes Authors.

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

package osinfo

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

const (
	OSDebian     = "debian"
	OSUbuntu     = "ubuntu"
	OSFedora     = "fedora"
	OSCentos     = "centos"
	OSRHEL       = "rhel"
	OSAlpine     = "alpine"
	OSWolfi      = "wolfi"
	OSDistroless = "distroless"
)

// TODO: Move functions to its own implementation
type LayerScanner struct{}

func (loss *LayerScanner) OSType(layerPath string) (ostype string, err error) {
	osrelease, err := loss.OSReleaseData(layerPath)
	if err != nil {
		if _, ok := err.(ErrFileNotFoundInTar); ok {
			return "", nil
		}
		if strings.Contains(err.Error(), "file not found") {
			return "", nil
		}
		return "", fmt.Errorf("reading os release: %w", err)
	}

	if osrelease == "" {
		return "", nil
	}

	if strings.Contains(osrelease, "NAME=\"Debian GNU") {
		logrus.Infof("Scan of container layers found %s base image", OSDebian)
		return OSDebian, nil
	}

	if strings.Contains(osrelease, "NAME=\"Ubuntu\"") {
		return OSUbuntu, nil
	}

	if strings.Contains(osrelease, "NAME=\"Fedora Linux\"") {
		return OSFedora, nil
	}

	if strings.Contains(osrelease, "NAME=\"CentOS Linux\"") {
		return OSCentos, nil
	}

	if strings.Contains(osrelease, "NAME=\"Red Hat Enterprise Linux\"") {
		return OSRHEL, nil
	}

	if strings.Contains(osrelease, "NAME=\"Alpine Linux\"") {
		return OSAlpine, nil
	}

	if strings.Contains(osrelease, "NAME=\"Wolfi\"") {
		return OSWolfi, nil
	}

	if strings.Contains(osrelease, "PRETTY_NAME=\"Distroless") {
		return OSDistroless, nil
	}
	return "", nil
}

// CanHandle looks at an image tarball and checks if it
// looks like a debian filesystem
func (loss *LayerScanner) OSReleaseData(layerPath string) (osrelease string, err error) {
	f, err := os.CreateTemp("", "os-release-")
	if err != nil {
		return osrelease, fmt.Errorf("creating temp file: %w", err)
	}
	defer f.Close()
	defer os.Remove(f.Name())

	destPath := f.Name()
	if err := loss.extractFileFromTar(layerPath, "etc/os-release", destPath); err != nil {
		return "", fmt.Errorf("extracting os-release from tar: %w", err)
	}
	if err != nil {
		return osrelease, err
	}
	data, err := os.ReadFile(destPath)
	if err != nil {
		return osrelease, fmt.Errorf("reading osrelease: %w", err)
	}
	return string(data), nil
}

type ErrFileNotFoundInTar struct{}

func (e ErrFileNotFoundInTar) Error() string {
	return "file not found in tarball"
}

// extractFileFromTar extracts filePath from tarPath and stores it in destPath
func (loss *LayerScanner) extractFileFromTar(tarPath, filePath, destPath string) error {
	// Open the tar file
	f, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("opening tarball: %w", err)
	}
	defer f.Close()

	// Read the first bytes to determine if the file is compressed
	var sample [3]byte
	var gzipped bool
	if _, err := io.ReadFull(f, sample[:]); err != nil {
		return fmt.Errorf("sampling bytes from file header: %w", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return fmt.Errorf("rewinding read pointer: %w", err)
	}

	// From: https://github.com/golang/go/blob/1fadc392ccaefd76ef7be5b685fb3889dbee27c6/src/compress/gzip/gunzip.go#L185
	if sample[0] == 0x1f && sample[1] == 0x8b && sample[2] == 0x08 {
		gzipped = true
	}

	const dotSl = "./"
	filePath = strings.TrimPrefix(filePath, dotSl)

	var tr *tar.Reader
	tr = tar.NewReader(f)
	if gzipped {
		gzf, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("creating gzip reader: %w", err)
		}
		tr = tar.NewReader(gzf)
	}

	// Search for the os-file in the tar contents
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return ErrFileNotFoundInTar{}
		}
		if err != nil {
			return fmt.Errorf("reading tarfile: %w", err)
		}

		if hdr.FileInfo().IsDir() {
			continue
		}

		// Scan for the os-release file in the tarball
		if strings.TrimPrefix(hdr.Name, dotSl) == filePath {
			// If this is a symlink, follow:
			if hdr.FileInfo().Mode()&os.ModeSymlink == os.ModeSymlink {
				target := hdr.Linkname
				// Check if its relative:
				if !strings.HasPrefix(target, string(filepath.Separator)) {
					newTarget := filepath.Dir(filePath)

					//nolint:gosec // This is not zipslip, path it not used for writing just
					// to search a file in the tarfile, the extract path is fexed.
					newTarget = filepath.Join(newTarget, hdr.Linkname)
					target = filepath.Clean(newTarget)
				}
				logrus.Infof("%s is a symlink, following to %s", filePath, target)
				return loss.extractFileFromTar(tarPath, target, destPath)
			}

			// Open the destination file
			destPointer, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("opening destination file: %w", err)
			}
			defer destPointer.Close()
			logrus.Infof("Writing %s to %s", filePath, destPath)
			for {
				if _, err = io.CopyN(destPointer, tr, 1024); err != nil {
					if err == io.EOF {
						return nil
					}
					return fmt.Errorf("writing data to %s: %w", destPath, err)
				}
			}
		}
	}
}
