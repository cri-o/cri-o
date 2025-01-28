package linklogs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/validation"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

const (
	kubeletPodsRootDir    = "/var/lib/kubelet/pods"
	kubeletPodLogsRootDir = "/var/log/pods"
	kubeletEmptyDirLogDir = "kubernetes.io~empty-dir"
)

// MountPodLogs bind mounts the kubelet pod log directory under the specified empty dir volume.
func MountPodLogs(ctx context.Context, kubePodUID, emptyDirVolName, namespace, kubeName, mountLabel string) error {
	// Validate the empty dir volume name
	// This uses the same validation as the one in kubernetes
	// It can be alphanumeric with dashes allowed in between
	if errs := validation.IsDNS1123Label(emptyDirVolName); len(errs) != 0 {
		return errors.New("empty dir vol name is invalid")
	}

	emptyDirLoggingVolumePath := podEmptyDirPath(kubePodUID, emptyDirVolName)
	if _, err := os.Stat(emptyDirLoggingVolumePath); err != nil {
		return fmt.Errorf("failed to find %v: %w", emptyDirLoggingVolumePath, err)
	}

	podLogsDirectory := namespace + "_" + kubeName + "_" + kubePodUID
	podLogsPath := filepath.Join(kubeletPodLogsRootDir, podLogsDirectory)
	log.Infof(ctx, "Mounting from %s to %s for linked logs", podLogsPath, emptyDirLoggingVolumePath)

	if err := unix.Mount(podLogsPath, emptyDirLoggingVolumePath, "bind", unix.MS_BIND|unix.MS_RDONLY, ""); err != nil {
		return fmt.Errorf("failed to mount %v to %v: %w", podLogsPath, emptyDirLoggingVolumePath, err)
	}

	if err := label.SetFileLabel(emptyDirLoggingVolumePath, mountLabel); err != nil {
		return fmt.Errorf("failed to set selinux label: %w", err)
	}

	return nil
}

// UnmountPodLogs unmounts the pod log directory from the specified empty dir volume.
func UnmountPodLogs(ctx context.Context, kubePodUID, emptyDirVolName string) error {
	emptyDirLoggingVolumePath := podEmptyDirPath(kubePodUID, emptyDirVolName)
	log.Infof(ctx, "Unmounting %s for linked logs", emptyDirLoggingVolumePath)

	if _, err := os.Stat(emptyDirLoggingVolumePath); !os.IsNotExist(err) {
		if err := unix.Unmount(emptyDirLoggingVolumePath, unix.MNT_DETACH); err != nil {
			return fmt.Errorf("failed to unmounts logs: %w", err)
		}
	}

	return nil
}

func LinkContainerLogs(ctx context.Context, kubePodUID, emptyDirVolName, id string, metadata *types.ContainerMetadata) error {
	emptyDirLoggingVolumePath := podEmptyDirPath(kubePodUID, emptyDirVolName)
	// Symlink a relative path so the location is legitimate inside and outside the container.
	from := fmt.Sprintf("%s/%d.log", metadata.Name, metadata.Attempt)
	to := filepath.Join(emptyDirLoggingVolumePath, id+".log")
	log.Infof(ctx, "Symlinking from %s to %s for linked logs", from, to)

	return os.Symlink(from, to)
}

func podEmptyDirPath(podUID, emptyDirVolName string) string {
	return filepath.Join(kubeletPodsRootDir, podUID, "volumes", kubeletEmptyDirLogDir, emptyDirVolName)
}
