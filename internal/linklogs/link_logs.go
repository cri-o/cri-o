package linklogs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/opencontainers/selinux/go-selinux/label"
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

	emptyDirLoggingVolumePath, err := podEmptyDirPath(kubePodUID, emptyDirVolName)
	if err != nil {
		return fmt.Errorf("failed to get empty dir path: %w", err)
	}

	if _, err := os.Stat(emptyDirLoggingVolumePath); err != nil {
		return fmt.Errorf("failed to find %v: %w", emptyDirLoggingVolumePath, err)
	}

	podLogsDirectory := namespace + "_" + kubeName + "_" + kubePodUID

	podLogsPath, err := securejoin.SecureJoin(kubeletPodLogsRootDir, podLogsDirectory)
	if err != nil {
		return fmt.Errorf("failed to join %v and %v: %w", kubeletPodLogsRootDir, podLogsDirectory, err)
	}

	log.Infof(ctx, "Mounting from %s to %s for linked logs", podLogsPath, emptyDirLoggingVolumePath)

	if err := mountLogPath(podLogsPath, emptyDirLoggingVolumePath); err != nil {
		return fmt.Errorf("failed to mount %v to %v: %w", podLogsPath, emptyDirLoggingVolumePath, err)
	}

	if err := label.SetFileLabel(emptyDirLoggingVolumePath, mountLabel); err != nil {
		return fmt.Errorf("failed to set selinux label: %w", err)
	}

	return nil
}

// UnmountPodLogs unmounts the pod log directory from the specified empty dir volume.
func UnmountPodLogs(ctx context.Context, kubePodUID, emptyDirVolName string) error {
	emptyDirLoggingVolumePath, err := podEmptyDirPath(kubePodUID, emptyDirVolName)
	if err != nil {
		return fmt.Errorf("failed to get empty dir path: %w", err)
	}

	log.Infof(ctx, "Unmounting %s for linked logs", emptyDirLoggingVolumePath)

	if _, err := os.Stat(emptyDirLoggingVolumePath); !os.IsNotExist(err) {
		if err := unmountLogPath(emptyDirLoggingVolumePath); err != nil {
			return fmt.Errorf("failed to unmounts logs: %w", err)
		}
	}

	return nil
}

func LinkContainerLogs(ctx context.Context, kubePodUID, emptyDirVolName, id string, metadata *types.ContainerMetadata) error {
	emptyDirLoggingVolumePath, err := podEmptyDirPath(kubePodUID, emptyDirVolName)
	if err != nil {
		return fmt.Errorf("failed to get empty dir path: %w", err)
	}

	// Symlink a relative path so the location is legitimate inside and outside the container.
	from := fmt.Sprintf("%s/%d.log", metadata.Name, metadata.Attempt)

	to, err := securejoin.SecureJoin(emptyDirLoggingVolumePath, id+".log")
	if err != nil {
		return fmt.Errorf("failed to join %v and %v: %w", emptyDirLoggingVolumePath, id+".log", err)
	}

	log.Infof(ctx, "Symlinking from %s to %s for linked logs", from, to)

	return os.Symlink(from, to)
}

func podEmptyDirPath(podUID, emptyDirVolName string) (string, error) {
	relativePath := strings.Join([]string{podUID, "volumes", kubeletEmptyDirLogDir, emptyDirVolName}, "/")

	dirPath, err := securejoin.SecureJoin(kubeletPodsRootDir, relativePath)
	if err != nil {
		return "", fmt.Errorf("failed to join %v and %v: %w", kubeletPodsRootDir, relativePath, err)
	}

	return dirPath, err
}
