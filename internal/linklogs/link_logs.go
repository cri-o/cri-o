package linklogs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/opencontainers/selinux/go-selinux/label"
	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	kubeletPodsRootDir    = "/var/lib/kubelet/pods"
	kubeletPodLogsRootDir = "/var/log/pods"
	kubeletEmptyDirLogDir = "kubernetes.io~empty-dir"
)

// MountPodLogs bind mounts the kubelet pod log directory under the specified empty dir volume
func MountPodLogs(ctx context.Context, kubePodUID, emptyDirVolName, namespace, kubeName, mountLabel string) error {
	// Validate the empty dir volume name
	// This uses the same validation as the one in kubernetes
	// It can be alphanumeric with dashes allowed in between
	if errs := validation.IsDNS1123Label(emptyDirVolName); len(errs) != 0 {
		return fmt.Errorf("empty dir vol name is invalid")
	}
	emptyDirLoggingVolumePath := podEmptyDirPath(kubePodUID, emptyDirVolName)
	if _, err := os.Stat(emptyDirLoggingVolumePath); err != nil {
		return fmt.Errorf("failed to find %v: %w", emptyDirLoggingVolumePath, err)
	}
	logDirMountPath := filepath.Join(emptyDirLoggingVolumePath, "logs")
	if err := os.Mkdir(logDirMountPath, 0o755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}
	podLogsDirectory := namespace + "_" + kubeName + "_" + kubePodUID
	podLogsPath := filepath.Join(kubeletPodLogsRootDir, podLogsDirectory)
	log.Infof(ctx, "Mounting from %s to %s for linked logs", podLogsPath, logDirMountPath)
	if err := unix.Mount(podLogsPath, logDirMountPath, "bind", unix.MS_BIND|unix.MS_RDONLY, ""); err != nil {
		return fmt.Errorf("failed to mount %v to %v: %w", podLogsPath, logDirMountPath, err)
	}
	if err := label.SetFileLabel(logDirMountPath, mountLabel); err != nil {
		return fmt.Errorf("failed to set selinux label: %w", err)
	}
	return nil
}

// UnmountPodLogs unmounts the pod log directory from the specified empty dir volume
func UnmountPodLogs(ctx context.Context, kubePodUID, emptyDirVolName string) error {
	emptyDirLoggingVolumePath := podEmptyDirPath(kubePodUID, emptyDirVolName)
	logDirMountPath := filepath.Join(emptyDirLoggingVolumePath, "logs")
	log.Infof(ctx, "Unmounting %s for linked logs", logDirMountPath)
	if _, err := os.Stat(logDirMountPath); !os.IsNotExist(err) {
		if err := unix.Unmount(logDirMountPath, unix.MNT_DETACH); err != nil {
			return fmt.Errorf("failed to unmounts logs: %w", err)
		}
	}
	return nil
}

func podEmptyDirPath(podUID, emptyDirVolName string) string {
	return filepath.Join(kubeletPodsRootDir, podUID, "volumes", kubeletEmptyDirLogDir, emptyDirVolName)
}
