package server

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	metadata "github.com/checkpoint-restore/checkpointctl/lib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
)

// RestorePod restores a pod sandbox from a checkpoint.
func (s *Server) RestorePod(ctx context.Context, req *types.RestorePodRequest) (*types.RestorePodResponse, error) {
	if !s.config.CheckpointRestore() {
		return nil, errors.New("checkpoint/restore support not available")
	}

	// Validate that path is provided
	if req.GetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "path is required for pod restore")
	}

	log.Infof(ctx, "Restoring pod from checkpoint: %s", req.GetPath())

	// Check if the path refers to a pod checkpoint OCI image
	podCheckpoint, err := s.checkIfPodCheckpointOCIImage(ctx, req.GetPath())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check checkpoint image: %v", err)
	}

	if podCheckpoint == nil {
		return nil, status.Errorf(codes.InvalidArgument, "path %q does not refer to a pod checkpoint image", req.GetPath())
	}

	log.Infof(ctx, "Found pod checkpoint for %q (namespace: %s, old ID: %s, UID: %s) in %s", podCheckpoint.PodName, podCheckpoint.PodNamespace, podCheckpoint.OldPodID, podCheckpoint.PodUID, req.GetPath())

	// Mount the checkpoint image to read its contents
	imageIDString := podCheckpoint.ImageID.IDStringForOutOfProcessConsumptionOnly()
	store := s.ContainerServer.StorageImageServer().GetStore()

	mountPoint, err := store.MountImage(imageIDString, nil, "")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to mount checkpoint image: %v", err)
	}
	defer func() {
		if _, err := store.UnmountImage(imageIDString, true); err != nil {
			log.Errorf(ctx, "Failed to unmount checkpoint image: %v", err)
		}
	}()

	log.Debugf(ctx, "Mounted checkpoint image at %s", mountPoint)

	// Read pod.options file to get the list of containers
	checkpointedPodOptions, _, err := metadata.ReadContainerCheckpointPodOptions(mountPoint)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to read pod options: %v", err)
	}

	if checkpointedPodOptions.Version != 1 {
		return nil, status.Errorf(codes.InvalidArgument, "unsupported pod checkpoint version %d", checkpointedPodOptions.Version)
	}

	log.Infof(ctx, "Pod checkpoint contains %d containers", len(checkpointedPodOptions.Containers))

	if len(checkpointedPodOptions.Containers) == 0 {
		return nil, status.Error(codes.InvalidArgument, "pod checkpoint contains no containers")
	}

	// Construct a PodSandboxConfig from checkpoint metadata
	// Use the provided config from request if available, otherwise construct from checkpoint
	var podConfig *types.PodSandboxConfig
	if req.GetConfig() != nil {
		podConfig = req.GetConfig()
		log.Infof(ctx, "Using provided PodSandboxConfig from request")
	} else {
		// Extract pod metadata from checkpoint annotations
		podConfig = &types.PodSandboxConfig{
			Metadata: &types.PodSandboxMetadata{
				Name:      podCheckpoint.PodName,
				Namespace: podCheckpoint.PodNamespace,
				Uid:       podCheckpoint.PodUID,
			},
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		}
		log.Infof(ctx, "Constructed minimal PodSandboxConfig from checkpoint metadata (UID: %s)", podCheckpoint.PodUID)
	}

	// Extract the runtime handler from a container's checkpoint metadata.
	// The runtime handler is stored per-container in config.dump (OCIRuntime
	// field) during checkpoint; all containers in a pod share the same
	// runtime handler, so reading any one is sufficient.
	var runtimeHandler string

	for _, containerDirName := range checkpointedPodOptions.Containers {
		var containerConfig metadata.ContainerConfig
		if _, err := metadata.ReadJSONFile(&containerConfig, filepath.Join(mountPoint, containerDirName), metadata.ConfigDumpFile); err == nil {
			runtimeHandler = containerConfig.OCIRuntime
		}

		break
	}

	if runtimeHandler != "" {
		log.Infof(ctx, "Using runtime handler %q from checkpoint metadata", runtimeHandler)
	}

	// Create a new pod sandbox using RunPodSandbox
	log.Infof(ctx, "Creating new pod sandbox for restored pod")

	runPodReq := &types.RunPodSandboxRequest{
		Config:         podConfig,
		RuntimeHandler: runtimeHandler,
	}

	sandboxResp, err := s.RunPodSandbox(ctx, runPodReq)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create pod sandbox: %v", err)
	}

	newPodID := sandboxResp.GetPodSandboxId()
	log.Infof(ctx, "Created new pod sandbox with ID: %s", newPodID)

	return s.restorePodContainers(ctx, newPodID, mountPoint, podCheckpoint, req, checkpointedPodOptions)
}

// restorePodContainers handles the post-RunPodSandbox container restoration
// logic: importing each container from the checkpoint and starting them.
func (s *Server) restorePodContainers(
	ctx context.Context,
	newPodID string,
	mountPoint string,
	podCheckpoint *PodCheckpointInfo,
	req *types.RestorePodRequest,
	checkpointedPodOptions *metadata.CheckpointedPodOptions,
) (*types.RestorePodResponse, error) {
	// Get the sandbox object for container restoration
	sb := s.GetSandbox(newPodID)
	if sb == nil {
		return nil, status.Errorf(codes.Internal, "failed to get created sandbox %s", newPodID)
	}

	// Now restore each container into the new sandbox
	log.Infof(ctx, "Restoring %d containers into pod %s", len(checkpointedPodOptions.Containers), newPodID)

	var restoredContainers []string

	containerIndex := 0
	for containerName, containerDirName := range checkpointedPodOptions.Containers {
		containerIndex++
		containerDir := filepath.Join(mountPoint, containerDirName)

		// Read container metadata
		var containerConfig metadata.ContainerConfig
		if _, err := metadata.ReadJSONFile(&containerConfig, containerDir, metadata.ConfigDumpFile); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to read config for container %s: %v", containerDirName, err)
		}

		log.Infof(ctx, "Restoring container %d/%d: %s (name: %s, short: %s)", containerIndex, len(checkpointedPodOptions.Containers), containerConfig.ID, containerConfig.Name, containerName)

		// Construct a ContainerConfig for CRImportCheckpoint
		// The Image field will point to the containerDir, which contains the checkpoint data
		// CRImportCheckpoint now supports directory-based checkpoints
		createConfig := &types.ContainerConfig{
			Metadata: &types.ContainerMetadata{
				Name:    containerName,
				Attempt: 0,
			},
			Image: &types.ImageSpec{
				Image: containerDir, // Point to the directory containing checkpoint data
			},
			Linux: &types.LinuxContainerConfig{
				Resources:       &types.LinuxContainerResources{},
				SecurityContext: &types.LinuxContainerSecurityContext{},
			},
		}

		// Call CRImportCheckpoint which will:
		// 1. Detect that containerDir is a directory (new feature)
		// 2. Use it directly without mounting or extracting
		// 3. Create the container structure
		// 4. Restore the container from the checkpoint data
		containerID, err := s.CRImportCheckpoint(ctx, createConfig, sb, podCheckpoint.PodUID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to restore container %s: %v", containerConfig.Name, err)
		}

		// Debug: List checkpoint directory contents
		if entries, err := os.ReadDir(containerDir); err == nil {
			log.Debugf(ctx, "Checkpoint directory %s contents:", containerDir)
			for _, entry := range entries {
				info, _ := entry.Info()
				if info != nil {
					log.Debugf(ctx, "  - %s (size: %d bytes, dir: %v)", entry.Name(), info.Size(), entry.IsDir())
				} else {
					log.Debugf(ctx, "  - %s (dir: %v)", entry.Name(), entry.IsDir())
				}
			}
		} else {
			log.Debugf(ctx, "Failed to list checkpoint directory %s: %v", containerDir, err)
		}

		log.Infof(ctx, "Successfully restored container %s with ID %s", containerConfig.Name, containerID)
		restoredContainers = append(restoredContainers, containerID)
	}

	log.Infof(ctx, "Successfully imported %d containers into pod %s: %v", len(restoredContainers), newPodID, restoredContainers)

	// Second loop: Start each container to trigger the actual CRIU restore
	// Containers are marked for restore, so StartContainer will call ContainerRestore
	log.Infof(ctx, "Starting CRIU restore for %d containers in pod %s", len(restoredContainers), newPodID)

	var startedContainers []string
	for i, containerID := range restoredContainers {
		log.Infof(ctx, "Starting container %d/%d: %s", i+1, len(restoredContainers), containerID)

		startReq := &types.StartContainerRequest{
			ContainerId: containerID,
		}

		_, startErr := s.StartContainer(ctx, startReq)
		if startErr != nil {
			log.Errorf(ctx, "Failed to start/restore container %s in pod %s: %v", containerID, newPodID, startErr)

			// Best-effort rollback: stop containers that were already started
			for _, startedID := range startedContainers {
				log.Infof(ctx, "Rollback: stopping container %s in pod %s", startedID, newPodID)
				stopReq := &types.StopContainerRequest{
					ContainerId: startedID,
					Timeout:     0,
				}
				if _, stopErr := s.StopContainer(ctx, stopReq); stopErr != nil {
					log.Errorf(ctx, "Rollback: failed to stop container %s in pod %s: %v", startedID, newPodID, stopErr)
				}
			}

			// Best-effort rollback: stop and remove the sandbox
			log.Infof(ctx, "Rollback: stopping pod sandbox %s", newPodID)
			stopPodReq := &types.StopPodSandboxRequest{
				PodSandboxId: newPodID,
			}
			if _, stopErr := s.StopPodSandbox(ctx, stopPodReq); stopErr != nil {
				log.Errorf(ctx, "Rollback: failed to stop pod sandbox %s: %v", newPodID, stopErr)
			}

			log.Infof(ctx, "Rollback: removing pod sandbox %s", newPodID)
			removePodReq := &types.RemovePodSandboxRequest{
				PodSandboxId: newPodID,
			}
			if _, removeErr := s.RemovePodSandbox(ctx, removePodReq); removeErr != nil {
				log.Errorf(ctx, "Rollback: failed to remove pod sandbox %s: %v", newPodID, removeErr)
			}

			return nil, status.Errorf(codes.Internal, "failed to start/restore container %s in pod %s: %v", containerID, newPodID, startErr)
		}

		log.Infof(ctx, "Successfully restored and started container %s", containerID)
		startedContainers = append(startedContainers, containerID)
	}

	log.Infof(ctx, "Successfully restored pod %s with %d containers: %v", newPodID, len(restoredContainers), restoredContainers)

	return &types.RestorePodResponse{
		PodSandboxId: newPodID,
	}, nil
}
