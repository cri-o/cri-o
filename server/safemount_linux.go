package server

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cyphar/filepath-securejoin/pathrs-lite"
	"golang.org/x/sys/unix"
)

type safeMountInfo struct {
	// file is the open File.
	file *os.File

	// mountPoint is the mount point.
	mountPoint string
}

// Close releases the resources allocated with the safe mount info.
func (s *safeMountInfo) Close() {
	_ = unix.Unmount(s.mountPoint, unix.MNT_DETACH) //nolint: errcheck
	_ = s.file.Close()
}

// safeMountSubPath securely mounts a subpath inside a volume to a new temporary location.
// The function checks that the subpath is a valid subpath within the volume and that it
// does not escape the boundaries of the mount point (volume).
//
// The caller is responsible for closing the file descriptor and unmounting the subpath
// when it's no longer needed.
func safeMountSubPath(mountPoint, subpath, runDir string) (s *safeMountInfo, err error) {
	file, err := pathrs.OpenInRoot(mountPoint, subpath)
	if err != nil {
		return nil, err
	}

	// we need to always reference the file by its fd, that points inside the mountpoint.
	fname := fmt.Sprintf("/proc/self/fd/%d", int(file.Fd()))

	fi, err := os.Stat(fname)
	if err != nil {
		return nil, err
	}

	var npath string

	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return nil, fmt.Errorf("file %q is a symlink", filepath.Join(mountPoint, subpath))
	case fi.IsDir():
		npath, err = os.MkdirTemp(runDir, "subpath")
		if err != nil {
			return nil, err
		}
	default:
		tmp, err := os.CreateTemp(runDir, "subpath")
		if err != nil {
			return nil, err
		}

		tmp.Close()
		npath = tmp.Name()
	}

	if err := unix.Mount(fname, npath, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return nil, err
	}

	return &safeMountInfo{
		file:       file,
		mountPoint: npath,
	}, nil
}
