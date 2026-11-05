//go:build linux

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"github.com/opencontainers/cgroups"
	cstorage "go.podman.io/storage"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

const (
	podCheckpointRecoveryDir     = "pod-checkpoint-recovery"
	podCheckpointRecoveryVersion = 1
	podCheckpointCleanupTimeout  = 30 * time.Second
)

type podCheckpointRecoveryMarker struct {
	Version      int      `json:"version"`
	SandboxID    string   `json:"sandboxId"`
	CgroupParent string   `json:"cgroupParent"`
	ContainerIDs []string `json:"containerIds"`
	OutputPath   string   `json:"outputPath,omitempty"`
	OutputDevice uint64   `json:"outputDevice,omitempty"`
	OutputInode  uint64   `json:"outputInode,omitempty"`
	TasksResumed bool     `json:"tasksResumed,omitempty"`
}

func (s *Server) checkpointPod(
	ctx context.Context,
	request *types.CheckpointPodRequest,
) (_ *types.CheckpointPodResponse, retErr error) {
	if err := validateCheckpointInput(request.GetOutputPath()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid checkpoint output: %v", err)
	}

	if request.GetPodSandboxId() == "" {
		return nil, status.Error(codes.InvalidArgument, "pod sandbox ID is required")
	}

	if len(request.GetContainerIds()) == 0 ||
		len(request.GetContainerIds()) > podCheckpointMaxContainers {
		return nil, status.Error(
			codes.InvalidArgument,
			"between 1 and 1024 container IDs are required",
		)
	}

	sb, err := s.getPodSandboxFromRequest(ctx, request.GetPodSandboxId())
	if err != nil {
		return nil, status.Errorf(
			codes.NotFound,
			"find pod sandbox %q: %v",
			request.GetPodSandboxId(),
			err,
		)
	}

	if sb.ID() != request.GetPodSandboxId() {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"pod sandbox ID %q is not canonical",
			request.GetPodSandboxId(),
		)
	}

	runtimeType, err := s.Runtime().RuntimeType(sb.RuntimeHandler())
	if err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "resolve runtime handler: %v", err)
	}

	if runtimeType != libconfig.DefaultRuntimeType {
		return nil, status.Errorf(
			codes.Unimplemented,
			"Pod checkpoint does not support runtime type %q",
			runtimeType,
		)
	}

	if err := s.recoverPodCheckpointForSandbox(ctx, sb.ID()); err != nil {
		return nil, fmt.Errorf("recover previous Pod checkpoint: %w", err)
	}

	if err := validateEmptyCheckpointOutput(request.GetOutputPath()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid checkpoint output: %v", err)
	}

	seen := make(map[string]struct{}, len(request.GetContainerIds()))
	containers := make([]*oci.Container, 0, len(request.GetContainerIds()))

	containerConfigs := make(map[string]*types.ContainerConfig, len(request.GetContainerIds()))
	for _, requestedID := range request.GetContainerIds() {
		if requestedID == "" {
			return nil, status.Error(codes.InvalidArgument, "container IDs must not be empty")
		}

		if _, exists := seen[requestedID]; exists {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"container ID %q is duplicated",
				requestedID,
			)
		}

		seen[requestedID] = struct{}{}

		container, err := s.GetContainerFromShortID(ctx, requestedID)
		if err != nil {
			return nil, status.Errorf(codes.NotFound, "find container %q: %v", requestedID, err)
		}

		if container.ID() != requestedID {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"container ID %q is not canonical",
				requestedID,
			)
		}

		if container.Sandbox() != sb.ID() {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"container %q does not belong to pod sandbox %q",
				requestedID,
				sb.ID(),
			)
		}

		if err := s.Runtime().UpdateContainerStatus(ctx, container); err != nil {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"inspect container %q: %v",
				requestedID,
				err,
			)
		}

		if container.State().Status != oci.ContainerStateRunning {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"container %q is %s, expected running",
				requestedID,
				container.State().Status,
			)
		}

		if container.ImageID() == nil {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"container %q has no base image ID",
				requestedID,
			)
		}

		config, err := loadContainerConfig(container.Dir())
		if err != nil {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"load checkpoint metadata for container %q: %v",
				requestedID,
				err,
			)
		}

		name := config.GetMetadata().GetName()
		if name == "" {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"container %q has no checkpoint metadata name",
				requestedID,
			)
		}

		if _, exists := containerConfigs[name]; exists {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"multiple containers have checkpoint metadata name %q",
				name,
			)
		}

		containerConfigs[name] = config

		containers = append(containers, container)
	}

	release, err := s.reservePodCheckpoint(
		sb.ID(),
		request.GetOutputPath(),
		request.GetContainerIds(),
	)
	if err != nil {
		return nil, status.Errorf(codes.Aborted, "reserve Pod checkpoint: %v", err)
	}
	defer release()

	sb.StopMutex().RLock()
	defer sb.StopMutex().RUnlock()

	if sb.Stopped() {
		return nil, status.Errorf(codes.FailedPrecondition, "pod sandbox %q is stopped", sb.ID())
	}

	sandboxConfig, err := loadPodSandboxConfig(sb.InfraContainer().Dir())
	if err != nil {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"load sandbox checkpoint metadata: %v",
			err,
		)
	}

	marker := podCheckpointRecoveryMarker{
		Version:      podCheckpointRecoveryVersion,
		SandboxID:    sb.ID(),
		CgroupParent: sb.CgroupParent(),
		ContainerIDs: append([]string(nil), request.GetContainerIds()...),
	}

	marker.OutputPath, err = filepath.EvalSymlinks(request.GetOutputPath())
	if err != nil {
		return nil, err
	}

	output, err := openCheckpointDirectory(marker.OutputPath)
	if err != nil {
		return nil, err
	}

	var outputStat unix.Stat_t

	statErr := unix.Fstat(int(output.Fd()), &outputStat)
	output.Close()

	if statErr != nil {
		return nil, statErr
	}

	marker.OutputDevice = outputStat.Dev
	marker.OutputInode = outputStat.Ino

	cgroupManager, err := s.config.CgroupManager().SandboxCgroupManager(sb.CgroupParent(), sb.ID())
	if err != nil {
		return nil, fmt.Errorf("load Pod cgroup: %w", err)
	}

	freezerState, err := cgroupManager.GetFreezerState()
	if err != nil {
		return nil, fmt.Errorf("inspect Pod cgroup freezer: %w", err)
	}

	if freezerState != cgroups.Thawed {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"pod sandbox cgroup is %s, expected THAWED",
			freezerState,
		)
	}

	if err := s.writePodCheckpointRecoveryMarker(marker); err != nil {
		return nil, fmt.Errorf("write Pod checkpoint recovery marker: %w", err)
	}

	markerActive := true
	defer func() {
		if !markerActive {
			return
		}

		cleanupCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			podCheckpointCleanupTimeout,
		)
		defer cancel()

		if err := s.recoverPodCheckpointMarker(cleanupCtx, &marker); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("recover Pod after checkpoint: %w", err))
		}
	}()

	containerDir := filepath.Join(request.GetOutputPath(), podCheckpointContainerDir)
	if err := os.Mkdir(containerDir, 0o700); err != nil {
		return nil, fmt.Errorf("create checkpoint container directory: %w", err)
	}

	if err := writeCheckpointJSON(
		request.GetOutputPath(),
		podCheckpointConfigFile,
		sandboxConfig,
	); err != nil {
		return nil, fmt.Errorf("write Pod checkpoint config: %w", err)
	}

	if err := cgroupManager.Freeze(cgroups.Frozen); err != nil {
		return nil, fmt.Errorf("freeze Pod cgroup: %w", err)
	}

	for _, container := range containers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if err := s.Runtime().PauseContainer(ctx, container); err != nil {
			return nil, fmt.Errorf("pause container %q: %w", container.ID(), err)
		}

		if err := s.Runtime().UpdateContainerStatus(ctx, container); err != nil {
			return nil, fmt.Errorf("verify paused container %q: %w", container.ID(), err)
		}

		if container.State().Status != oci.ContainerStatePaused {
			return nil, fmt.Errorf(
				"container %q is %s after pause, expected paused",
				container.ID(),
				container.State().Status,
			)
		}
	}

	if err := cgroupManager.Freeze(cgroups.Thawed); err != nil {
		return nil, fmt.Errorf("thaw Pod cgroup after pausing containers: %w", err)
	}

	manifest := podCheckpointManifest{
		Version:        podCheckpointFormatVersion,
		Runtime:        "cri-o",
		RuntimeHandler: s.resolvedRuntimeHandler(sb.RuntimeHandler()),
		SandboxID:      sb.ID(),
		Containers:     make([]podCheckpointManifestContainer, 0, len(containers)),
	}
	checkpointConfigs := make([]*metadata.ContainerConfig, 0, len(containers))
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			podCheckpointCleanupTimeout,
		)
		defer cancel()
		for _, config := range checkpointConfigs {
			if err := s.CleanupContainerCheckpoint(cleanupCtx, config); err != nil {
				log.Errorf(
					ctx,
					"Unable to clean checkpoint artifacts for container %q: %v",
					config.ID,
					err,
				)
			}
		}
	}()
	for _, container := range containers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		config := &metadata.ContainerConfig{ID: container.ID()}
		checkpointConfigs = append(checkpointConfigs, config)
		_, err := s.ContainerCheckpoint(ctx, config, &lib.ContainerCheckpointOptions{
			KeepRunning: true,
			TargetFile: filepath.Join(
				request.GetOutputPath(),
				checkpointArchiveName(container.ID()),
			) + ".partial",
			ContainerIsPaused: true,
			DeferArchive:      true,
		})
		if err != nil {
			return nil, fmt.Errorf("checkpoint container %q: %w", container.ID(), err)
		}
	}

	cleanupCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		podCheckpointCleanupTimeout,
	)
	if err := s.resumeCheckpointContainers(cleanupCtx, cgroupManager, containers); err != nil {
		cancel()

		return nil, fmt.Errorf("resume Pod after checkpoint: %w", err)
	}

	cancel()

	marker.TasksResumed = true
	if err := s.persistPodCheckpointRecoveryMarker(&marker); err != nil {
		return nil, fmt.Errorf("record resumed Pod checkpoint: %w", err)
	}

	// The CRIU images and rootfs diff are immutable now, so creating and
	// hashing the outer archives does not require the Pod to remain paused.
	for i, container := range containers {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		archiveName := checkpointArchiveName(container.ID())
		archivePath := filepath.Join(request.GetOutputPath(), archiveName)
		temporaryPath := archivePath + ".partial"
		if err := s.ExportContainerCheckpoint(
			ctx,
			checkpointConfigs[i],
			temporaryPath,
		); err != nil {
			return nil, fmt.Errorf("export checkpoint for container %q: %w", container.ID(), err)
		}
		if err := syncCheckpointFile(temporaryPath); err != nil {
			return nil, err
		}
		if err := os.Rename(temporaryPath, archivePath); err != nil {
			return nil, fmt.Errorf(
				"publish checkpoint archive for container %q: %w",
				container.ID(),
				err,
			)
		}
		digest, err := fileSHA256(archivePath)
		if err != nil {
			return nil, fmt.Errorf(
				"hash checkpoint archive for container %q: %w",
				container.ID(),
				err,
			)
		}

		name := container.Metadata().GetName()
		manifest.Containers = append(manifest.Containers, podCheckpointManifestContainer{
			Name:    name,
			ID:      container.ID(),
			Archive: archiveName,
			SHA256:  digest,
			Config:  containerConfigs[name],
			ImageID: container.ImageID().IDStringForOutOfProcessConsumptionOnly(),
		})
	}

	if err := syncCheckpointDirectory(containerDir); err != nil {
		return nil, fmt.Errorf("persist checkpoint archives: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := writeCheckpointJSON(
		request.GetOutputPath(),
		podCheckpointManifestFile,
		&manifest,
	); err != nil {
		return nil, fmt.Errorf("write Pod checkpoint manifest: %w", err)
	}

	if err := s.removePodCheckpointRecoveryMarker(sb.ID()); err != nil {
		return nil, fmt.Errorf("remove Pod checkpoint recovery marker: %w", err)
	}

	markerActive = false

	return &types.CheckpointPodResponse{}, nil
}

func (s *Server) resolvedRuntimeHandler(handler string) string {
	if handler == "" {
		return s.config.DefaultRuntime
	}

	return handler
}

func (s *Server) resumeCheckpointContainers(
	ctx context.Context,
	manager cgroups.Manager,
	containers []*oci.Container,
) error {
	var recoveryErrors []error
	if err := manager.Freeze(cgroups.Thawed); err != nil {
		recoveryErrors = append(recoveryErrors, fmt.Errorf("thaw Pod cgroup: %w", err))
	}

	for _, container := range containers {
		if err := s.Runtime().UpdateContainerStatus(ctx, container); err != nil {
			recoveryErrors = append(
				recoveryErrors,
				fmt.Errorf("inspect container %q during recovery: %w", container.ID(), err),
			)

			continue
		}

		if container.State().Status == oci.ContainerStatePaused {
			if err := s.Runtime().UnpauseContainer(ctx, container); err != nil {
				recoveryErrors = append(
					recoveryErrors,
					fmt.Errorf("resume container %q: %w", container.ID(), err),
				)

				continue
			}
		}

		if err := s.ContainerStateToDisk(ctx, container); err != nil {
			recoveryErrors = append(
				recoveryErrors,
				fmt.Errorf("persist recovered container %q: %w", container.ID(), err),
			)
		}
	}

	return errors.Join(recoveryErrors...)
}

func syncCheckpointFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open checkpoint archive for sync: %w", err)
	}
	defer file.Close()

	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync checkpoint archive: %w", err)
	}

	return nil
}

func (s *Server) podCheckpointRecoveryDirectory() string {
	return filepath.Join(s.Store().GraphRoot(), "crio", podCheckpointRecoveryDir)
}

func (s *Server) podCheckpointRecoveryMarkerPath(sandboxID string) string {
	digest := sha256.Sum256([]byte(sandboxID))

	return filepath.Join(s.podCheckpointRecoveryDirectory(), hex.EncodeToString(digest[:])+".json")
}

func (s *Server) writePodCheckpointRecoveryMarker(marker podCheckpointRecoveryMarker) error {
	dir := s.podCheckpointRecoveryDirectory()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	if _, err := os.Lstat(s.podCheckpointRecoveryMarkerPath(marker.SandboxID)); err == nil {
		return fmt.Errorf(
			"pod checkpoint recovery marker for sandbox %q already exists",
			marker.SandboxID,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return s.persistPodCheckpointRecoveryMarker(&marker)
}

func (s *Server) persistPodCheckpointRecoveryMarker(marker *podCheckpointRecoveryMarker) error {
	return writeCheckpointJSON(
		s.podCheckpointRecoveryDirectory(),
		filepath.Base(s.podCheckpointRecoveryMarkerPath(marker.SandboxID)),
		marker,
	)
}

func (s *Server) removePodCheckpointRecoveryMarker(sandboxID string) error {
	path := s.podCheckpointRecoveryMarkerPath(sandboxID)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return syncCheckpointDirectory(filepath.Dir(path))
}

func (s *Server) recoverPodCheckpoints(ctx context.Context) error {
	dir := s.podCheckpointRecoveryDirectory()

	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if entry.Name()[0] == '.' {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}

			continue
		}

		marker := new(podCheckpointRecoveryMarker)

		path := filepath.Join(dir, entry.Name())
		if err := readCheckpointJSON(path, marker); err != nil {
			return err
		}

		if marker.Version != podCheckpointRecoveryVersion || marker.SandboxID == "" ||
			len(marker.ContainerIDs) == 0 {
			return fmt.Errorf("invalid Pod checkpoint recovery marker %s", path)
		}

		if path != s.podCheckpointRecoveryMarkerPath(marker.SandboxID) {
			return fmt.Errorf("pod checkpoint recovery marker %s has an invalid filename", path)
		}

		if err := s.recoverPodCheckpointMarker(ctx, marker); err != nil {
			return err
		}
	}

	return syncCheckpointDirectory(dir)
}

// Retry a retained marker before accepting another checkpoint of this sandbox.
// Startup recovery runs before requests and does not need these reservations.
func (s *Server) recoverPodCheckpointForSandbox(ctx context.Context, sandboxID string) error {
	path := s.podCheckpointRecoveryMarkerPath(sandboxID)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	if _, loaded := s.podCheckpointsInProgress.LoadOrStore(sandboxID, struct{}{}); loaded {
		return status.Error(codes.Aborted, "a checkpoint of this sandbox is already active")
	}
	defer s.podCheckpointsInProgress.Delete(sandboxID)

	var marker podCheckpointRecoveryMarker
	if err := readCheckpointJSON(path, &marker); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}

	if marker.SandboxID != sandboxID || marker.Version != podCheckpointRecoveryVersion ||
		len(marker.ContainerIDs) == 0 {
		return errors.New("invalid Pod checkpoint recovery marker")
	}

	if marker.OutputPath != "" {
		if _, loaded := s.podCheckpointOutputs.LoadOrStore(marker.OutputPath, struct{}{}); loaded {
			return status.Error(codes.Aborted, "checkpoint output is in use")
		}
		defer s.podCheckpointOutputs.Delete(marker.OutputPath)
	}

	releaseContainers, err := s.reserveContainerCheckpoints(marker.ContainerIDs)
	if err != nil {
		return status.Errorf(codes.Aborted, "reserve checkpoint recovery: %v", err)
	}
	defer releaseContainers()

	if sb := s.GetSandbox(sandboxID); sb != nil {
		sb.StopMutex().RLock()
		defer sb.StopMutex().RUnlock()
	}

	return s.recoverPodCheckpointMarker(ctx, &marker)
}

func (s *Server) recoverPodCheckpointMarker(
	ctx context.Context,
	marker *podCheckpointRecoveryMarker,
) error {
	if !marker.TasksResumed {
		sb := s.GetSandbox(marker.SandboxID)
		if sb == nil {
			for _, id := range append([]string{marker.SandboxID}, marker.ContainerIDs...) {
				if _, err := s.Store().Container(id); err == nil {
					return fmt.Errorf("checkpoint resource %q has not been loaded", id)
				} else if !errors.Is(err, cstorage.ErrContainerUnknown) {
					return err
				}
			}
		} else {
			if marker.CgroupParent != sb.CgroupParent() {
				return errors.New("checkpoint marker cgroup does not match the sandbox")
			}

			manager, err := s.config.CgroupManager().
				SandboxCgroupManager(marker.CgroupParent, marker.SandboxID)
			if err != nil {
				return err
			}

			containers := make([]*oci.Container, 0, len(marker.ContainerIDs))
			for _, id := range marker.ContainerIDs {
				container := s.GetContainer(ctx, id)
				if container == nil {
					if _, err := s.Store().Container(id); err == nil {
						return fmt.Errorf("checkpoint container %q has not been loaded", id)
					} else if !errors.Is(err, cstorage.ErrContainerUnknown) {
						return err
					}

					continue
				}

				if container.Sandbox() != marker.SandboxID {
					return errors.New("checkpoint marker names a container from another sandbox")
				}

				containers = append(containers, container)
			}

			if err := s.resumeCheckpointContainers(ctx, manager, containers); err != nil {
				return err
			}
		}

		marker.TasksResumed = true
		if err := s.persistPodCheckpointRecoveryMarker(marker); err != nil {
			return err
		}
	}

	if err := cleanupInterruptedPodCheckpoint(marker); err != nil {
		return err
	}

	return s.removePodCheckpointRecoveryMarker(marker.SandboxID)
}

func cleanupInterruptedPodCheckpoint(marker *podCheckpointRecoveryMarker) error {
	if marker.OutputPath == "" {
		return nil // Older markers did not record an output directory.
	}

	directory, err := openCheckpointDirectory(marker.OutputPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	defer directory.Close()

	var stat unix.Stat_t
	if err := unix.Fstat(int(directory.Fd()), &stat); err != nil {
		return err
	}

	if stat.Dev != marker.OutputDevice || stat.Ino != marker.OutputInode {
		return errors.New("checkpoint output directory was replaced")
	}

	var manifest podCheckpointManifest
	if err := readCheckpointJSONAt(directory, podCheckpointManifestFile, &manifest); err == nil {
		if manifest.Version != podCheckpointFormatVersion || manifest.Runtime != "cri-o" ||
			manifest.SandboxID != marker.SandboxID {
			return errors.New("published checkpoint does not match the recovery marker")
		}

		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	// The descriptor pins the original directory even if its pathname changes.
	root, err := os.OpenRoot(fmt.Sprintf("/proc/self/fd/%d", directory.Fd()))
	if err != nil {
		return err
	}
	defer root.Close()

	for _, name := range []string{podCheckpointConfigFile, podCheckpointContainerDir} {
		if err := root.RemoveAll(name); err != nil {
			return err
		}
	}

	entries, err := directory.ReadDir(-1)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "."+podCheckpointConfigFile+"-") ||
			strings.HasPrefix(entry.Name(), "."+podCheckpointManifestFile+"-") {
			if err := root.Remove(entry.Name()); err != nil {
				return err
			}
		}
	}

	return directory.Sync()
}
