/*
Copyright 2020 The Kubernetes Authors.

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

package tar

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

// Compress the provided  `tarContentsPath` into the `tarFilePath` while
// excluding the `exclude` regular expression patterns.
func Compress(tarFilePath, tarContentsPath string, excludes ...*regexp.Regexp) error {
	tarFile, err := os.Create(tarFilePath)
	if err != nil {
		return fmt.Errorf("create tar file %q: %w", tarFilePath, err)
	}
	defer tarFile.Close()

	gzipWriter := gzip.NewWriter(tarFile)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	if err := filepath.Walk(tarContentsPath, func(filePath string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		var link string
		isLink := fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink
		if isLink {
			link, err = os.Readlink(filePath)
			if err != nil {
				return fmt.Errorf("read file link of %s: %w", filePath, err)
			}
		}

		header, err := tar.FileInfoHeader(fileInfo, link)
		if err != nil {
			return fmt.Errorf("create file info header for %q: %w", filePath, err)
		}

		if fileInfo.IsDir() || filePath == tarFilePath {
			logrus.Tracef("Skipping: %s", filePath)
			return nil
		}

		for _, re := range excludes {
			if re != nil && re.MatchString(filePath) {
				logrus.Tracef("Excluding: %s", filePath)
				return nil
			}
		}

		// Make the path inside the tar relative to the archive path if
		// necessary.
		header.Name = strings.TrimLeft(
			strings.TrimPrefix(filePath, filepath.Dir(tarFilePath)),
			string(filepath.Separator),
		)
		header.Linkname = filepath.ToSlash(header.Linkname)

		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("writing tar header: %w", err)
		}

		if !isLink {
			file, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("open file %q: %w", filePath, err)
			}

			if _, err := io.Copy(tarWriter, file); err != nil {
				return fmt.Errorf("writing file to tar writer: %w", err)
			}

			file.Close()
		}

		return nil
	}); err != nil {
		return fmt.Errorf("walking tree in %q: %w", tarContentsPath, err)
	}

	return nil
}

// Extract can be used to extract the provided `tarFilePath` into the
// `destinationPath`.
func Extract(tarFilePath, destinationPath string) error {
	return iterateTarball(
		tarFilePath,
		func(reader *tar.Reader, header *tar.Header) (stop bool, err error) {
			switch header.Typeflag {
			case tar.TypeDir:
				targetDir := filepath.Join(destinationPath, header.Name)
				logrus.Tracef("Creating directory %s", targetDir)
				if err := os.Mkdir(targetDir, os.FileMode(0o755)); err != nil {
					return false, fmt.Errorf("create target directory: %w", err)
				}
			case tar.TypeSymlink:
				targetFile := filepath.Join(destinationPath, header.Name)
				logrus.Tracef(
					"Creating symlink %s -> %s", header.Linkname, targetFile,
				)
				if err := os.MkdirAll(
					filepath.Dir(targetFile), os.FileMode(0o755),
				); err != nil {
					return false, fmt.Errorf("create target directory: %w", err)
				}
				if err := os.Symlink(header.Linkname, targetFile); err != nil {
					return false, fmt.Errorf("create symlink: %w", err)
				}
				// tar.TypeRegA has been deprecated since Go 1.11
				// should we just remove?
			case tar.TypeReg, tar.TypeRegA: //nolint: staticcheck
				targetFile := filepath.Join(destinationPath, header.Name)
				logrus.Tracef("Creating file %s", targetFile)

				if err := os.MkdirAll(
					filepath.Dir(targetFile), os.FileMode(0o755),
				); err != nil {
					return false, fmt.Errorf("create target directory: %w", err)
				}

				outFile, err := os.Create(targetFile)
				if err != nil {
					return false, fmt.Errorf("create target file: %w", err)
				}
				if err := outFile.Chmod(os.FileMode(header.Mode)); err != nil {
					return false, fmt.Errorf("chmod target file: %w", err)
				}

				if _, err := io.Copy(outFile, reader); err != nil {
					return false, fmt.Errorf("copy file contents %s: %w", targetFile, err)
				}
				outFile.Close()

			default:
				logrus.Warnf(
					"File %s has unknown type %s",
					header.Name, string(header.Typeflag),
				)
			}

			return false, nil
		},
	)
}

// ReadFileFromGzippedTar opens a tarball and reads contents of a file inside.
func ReadFileFromGzippedTar(
	tarPath, filePath string,
) (res io.Reader, err error) {
	if err := iterateTarball(
		tarPath,
		func(reader *tar.Reader, header *tar.Header) (stop bool, err error) {
			if header.Name == filePath {
				res = reader
				return true, nil
			}
			return false, nil
		},
	); err != nil {
		return nil, err
	}

	if res == nil {
		return nil, fmt.Errorf("unable to find file %q in tarball %q: %w", tarPath, filePath, err)
	}

	return res, nil
}

// iterateTarball can be used to iterate over the contents of a tarball by
// calling the callback for each entry.
func iterateTarball(
	tarPath string,
	callback func(*tar.Reader, *tar.Header) (stop bool, err error),
) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("opening tar file %q: %w", tarPath, err)
	}

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("creating gzip reader for file %q: %w", tarPath, err)
	}
	tarReader := tar.NewReader(gzipReader)

	for {
		tarHeader, err := tarReader.Next()
		if err == io.EOF {
			break // End of archive
		}

		stop, err := callback(tarReader, tarHeader)
		if err != nil {
			return err
		}
		if stop {
			// User wants to stop
			break
		}
	}

	return nil
}
