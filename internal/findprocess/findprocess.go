// Package findprocess provides an os.FindProcess wrapper that
// portably detects non-existent processes.
package findprocess

import (
	"os"

	"github.com/pkg/errors"
)

// ErrNotFound represents a target process that does not exist or is
// otherwise not available to the calling process.
var ErrNotFound = errors.New("process not found")

// FindProcess wraps os.Findprocess [1] to return a public ErrNotFound
// if the process does not exist.  The returned process will be nil if
// and only if the returned err is non-nil.
//
// [1]: https://golang.org/pkg/os/#FindProcess
func FindProcess(pid int) (*os.Process, error) {
	process, err := findProcess(pid)
	if err != nil {
		releaseErr := process.Release()
		process = nil
		if releaseErr != nil {
			return process, errors.Wrap(err, releaseErr.Error())
		}
	}
	return process, err
}
