// Package auth contains commonly used auth file helper functions.
package auth

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path"
	"path/filepath"
)

// FilePath returns a path to the auth file for the provided auth directory
// (dir), namespace and imageRef. The resulting path has the following format:
// <dir>/<namespace>-<imageRef as SHA256>.json
//
// The function errors if:
// - dir is not an absolute path or not provided.
// - namespace is not provided.
// - imageRef is not provided.
func FilePath(dir, namespace, imageRef string) (string, error) {
	if !path.IsAbs(dir) {
		return "", fmt.Errorf("provided %q directory is not an absolute path", dir)
	}

	if namespace == "" {
		return "", errors.New("no namespace provided")
	}

	if imageRef == "" {
		return "", errors.New("no image ref provided")
	}

	hash := sha256.Sum256([]byte(imageRef))

	return filepath.Join(dir, fmt.Sprintf("%s-%x.json", namespace, hash)), nil
}
