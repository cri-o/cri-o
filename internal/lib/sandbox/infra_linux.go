package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/sys/unix"
)

// SetupShim mounts a path to pod sandbox's shared memory.
func SetupShm(podSandboxRunDir, mountLabel string, shmSize int64) (shmPath string, _ error) {
	if shmSize <= 0 {
		return "", fmt.Errorf("shm size %d must be greater than 0", shmSize)
	}

	shmPath = filepath.Join(podSandboxRunDir, "shm")
	if err := os.Mkdir(shmPath, 0o700); err != nil {
		return "", err
	}

	shmOptions := "mode=1777,size=" + strconv.FormatInt(shmSize, 10)
	if err := unix.Mount("shm", shmPath, "tmpfs", unix.MS_NOEXEC|unix.MS_NOSUID|unix.MS_NODEV,
		label.FormatMountLabel(shmOptions, mountLabel)); err != nil {
		return "", fmt.Errorf("failed to mount shm tmpfs for pod: %w", err)
	}

	return shmPath, nil
}
