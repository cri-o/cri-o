//go:build linux

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

const (
	podRestoreTransactionDir     = "pod-restore-transactions"
	podRestoreTransactionVersion = 1
)

type podRestoreTransaction struct {
	Version      int      `json:"version"`
	Key          string   `json:"key"`
	SandboxName  string   `json:"sandboxName"`
	SandboxID    string   `json:"sandboxId,omitempty"`
	ContainerIDs []string `json:"containerIds,omitempty"`
}

func (s *Server) restorePod(ctx context.Context, request *types.RestorePodRequest) (_ *types.RestorePodResponse, retErr error) {
	if err := validateCheckpointInput(request.GetCheckpointPath()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid checkpoint path: %v", err)
	}

	if request.GetConfig() == nil || request.GetConfig().GetMetadata() == nil {
		return nil, status.Error(codes.InvalidArgument, "sandbox config and metadata are required")
	}

	if len(request.GetContainerConfigs()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one container config is required")
	}

	runtimeType, err := s.Runtime().RuntimeType(request.GetRuntimeHandler())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "resolve runtime handler: %v", err)
	}

	if runtimeType != libconfig.DefaultRuntimeType {
		return nil, status.Errorf(codes.Unimplemented, "Pod restore does not support runtime type %q", runtimeType)
	}

	manifest := new(podCheckpointManifest)
	if err := readCheckpointJSON(filepath.Join(request.GetCheckpointPath(), podCheckpointManifestFile), manifest); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "read Pod checkpoint manifest: %v", err)
	}

	if manifest.Version != podCheckpointFormatVersion || manifest.Runtime != "cri-o" {
		return nil, status.Errorf(codes.FailedPrecondition, "unsupported Pod checkpoint format version %d for runtime %q", manifest.Version, manifest.Runtime)
	}

	restoreRuntimeHandler := s.resolvedRuntimeHandler(request.GetRuntimeHandler())
	if manifest.RuntimeHandler != restoreRuntimeHandler {
		return nil, status.Errorf(codes.FailedPrecondition, "restore runtime handler %q does not match checkpoint runtime handler %q", restoreRuntimeHandler, manifest.RuntimeHandler)
	}

	checkpointSandboxConfig := new(types.PodSandboxConfig)
	if err := readCheckpointJSON(filepath.Join(request.GetCheckpointPath(), podCheckpointConfigFile), checkpointSandboxConfig); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "read checkpoint sandbox config: %v", err)
	}

	if err := validateRestoreSandboxConfig(checkpointSandboxConfig, request.GetConfig()); err != nil {
		return nil, status.Errorf(codes.FailedPrecondition, "validate restore sandbox config: %v", err)
	}

	requestedByName := make(map[string]*types.ContainerConfig, len(request.GetContainerConfigs()))
	for _, config := range request.GetContainerConfigs() {
		name := config.GetMetadata().GetName()
		if name == "" {
			return nil, status.Error(codes.InvalidArgument, "every restore container config must have a metadata name")
		}

		if config.GetImage() == nil {
			return nil, status.Errorf(codes.InvalidArgument, "restore container %q has no image", name)
		}

		if _, exists := requestedByName[name]; exists {
			return nil, status.Errorf(codes.InvalidArgument, "restore container name %q is duplicated", name)
		}

		requestedByName[name] = config
	}

	if len(manifest.Containers) != len(requestedByName) {
		return nil, status.Error(codes.FailedPrecondition, "restore container set does not match checkpoint")
	}

	checkpointByName := make(map[string]podCheckpointManifestContainer, len(manifest.Containers))
	for _, container := range manifest.Containers {
		if container.Name == "" || container.ID == "" || container.Config == nil {
			return nil, status.Error(codes.InvalidArgument, "checkpoint manifest contains an incomplete container entry")
		}

		if _, exists := checkpointByName[container.Name]; exists {
			return nil, status.Errorf(codes.InvalidArgument, "checkpoint contains duplicate container name %q", container.Name)
		}

		if container.Archive != checkpointArchiveName(container.ID) {
			return nil, status.Errorf(codes.InvalidArgument, "checkpoint archive path for container %q is invalid", container.Name)
		}

		archivePath := filepath.Join(request.GetCheckpointPath(), container.Archive)

		info, err := os.Lstat(archivePath)
		if err != nil || !info.Mode().IsRegular() {
			return nil, status.Errorf(codes.InvalidArgument, "checkpoint archive for container %q is not a regular file", container.Name)
		}

		digest, err := fileSHA256(archivePath)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "hash checkpoint archive for container %q: %v", container.Name, err)
		}

		if digest != container.SHA256 {
			return nil, status.Errorf(codes.InvalidArgument, "checkpoint archive checksum for container %q does not match manifest", container.Name)
		}

		restoreConfig, exists := requestedByName[container.Name]
		if !exists {
			return nil, status.Errorf(codes.FailedPrecondition, "checkpoint container %q is missing from restore request", container.Name)
		}

		if err := validateRestoreContainerConfig(container.Config, restoreConfig); err != nil {
			return nil, status.Errorf(codes.FailedPrecondition, "validate restore config for container %q: %v", container.Name, err)
		}

		checkpointByName[container.Name] = container
	}

	restoreKey := fmt.Sprintf("%s/%s/%s", request.GetConfig().GetMetadata().GetNamespace(), request.GetConfig().GetMetadata().GetName(), request.GetConfig().GetMetadata().GetUid())
	if _, loaded := s.podRestoresInProgress.LoadOrStore(restoreKey, struct{}{}); loaded {
		return nil, status.Errorf(codes.Aborted, "restore for Pod %q is already in progress", restoreKey)
	}
	defer s.podRestoresInProgress.Delete(restoreKey)

	transaction := podRestoreTransaction{
		Version:     podRestoreTransactionVersion,
		Key:         restoreKey,
		SandboxName: makeSandboxContainerName(request.GetConfig()),
	}
	if _, err := os.Lstat(s.podRestoreTransactionPath(restoreKey)); os.IsNotExist(err) {
		if sandboxID, lookupErr := s.PodIDForName(transaction.SandboxName); lookupErr == nil {
			return s.existingPodRestoreResponse(sandboxID, restoreRuntimeHandler, request.GetContainerConfigs())
		}
	} else if err != nil {
		return nil, fmt.Errorf("inspect Pod restore transaction: %w", err)
	}

	for _, config := range request.GetContainerConfigs() {
		imageStatus, err := s.ImageStatus(ctx, &types.ImageStatusRequest{Image: config.GetImage()})
		if err != nil {
			return nil, fmt.Errorf("inspect image for container %q: %w", config.GetMetadata().GetName(), err)
		}

		if imageStatus.GetImage() == nil {
			if _, err := s.PullImage(ctx, &types.PullImageRequest{Image: config.GetImage(), SandboxConfig: request.GetConfig()}); err != nil {
				return nil, fmt.Errorf("pull image for container %q: %w", config.GetMetadata().GetName(), err)
			}
		}
	}

	if err := s.beginPodRestoreTransaction(ctx, &transaction); err != nil {
		return nil, fmt.Errorf("write Pod restore transaction: %w", err)
	}

	transactionActive := true
	defer func() {
		if !transactionActive {
			return
		}

		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), podCheckpointCleanupTimeout)
		defer cancel()

		if err := s.rollbackPodRestore(cleanupCtx, &transaction); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("roll back Pod restore transaction: %w", err))

			return
		}

		if err := s.removePodRestoreTransaction(restoreKey); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("remove Pod restore transaction: %w", err))
		}
	}()

	sandboxResponse, err := s.RunPodSandbox(ctx, &types.RunPodSandboxRequest{
		Config:         request.GetConfig(),
		RuntimeHandler: request.GetRuntimeHandler(),
	})
	if err != nil {
		return nil, fmt.Errorf("create restore sandbox: %w", err)
	}

	sandboxID := sandboxResponse.GetPodSandboxId()

	transaction.SandboxID = sandboxID
	if err := s.writePodRestoreTransaction(&transaction); err != nil {
		return nil, fmt.Errorf("record restore sandbox: %w", err)
	}

	response := &types.RestorePodResponse{PodSandboxId: sandboxID}

	for _, config := range request.GetContainerConfigs() {
		name := config.GetMetadata().GetName()

		createResponse, err := s.CreateContainer(ctx, &types.CreateContainerRequest{
			PodSandboxId:  sandboxID,
			Config:        config,
			SandboxConfig: request.GetConfig(),
		})
		if err != nil {
			return nil, fmt.Errorf("create restored container %q: %w", name, err)
		}

		container, err := s.GetContainerFromShortID(ctx, createResponse.GetContainerId())
		if err != nil {
			return nil, fmt.Errorf("load restored container %q: %w", name, err)
		}

		if container.State().Status != oci.ContainerStateCreated {
			return nil, fmt.Errorf("restored container %q is %s, expected created", name, container.State().Status)
		}

		checkpointContainer := checkpointByName[name]
		source := filepath.Join(request.GetCheckpointPath(), checkpointContainer.Archive)

		destination := filepath.Join(container.Dir(), podRestoreArchiveFile)
		if err := copyCheckpointArchive(source, destination); err != nil {
			return nil, fmt.Errorf("stage checkpoint for container %q: %w", name, err)
		}

		container.SetRestore(true)
		container.SetRestoreArchivePath(destination)

		if err := s.ContainerStateToDisk(ctx, container); err != nil {
			return nil, fmt.Errorf("persist restore state for container %q: %w", name, err)
		}

		transaction.ContainerIDs = append(transaction.ContainerIDs, container.ID())
		if err := s.writePodRestoreTransaction(&transaction); err != nil {
			return nil, fmt.Errorf("record restored container %q: %w", name, err)
		}

		response.RestoredContainers = append(response.RestoredContainers, &types.RestoredContainer{
			Name:        name,
			ContainerId: container.ID(),
		})
	}

	if err := s.removePodRestoreTransaction(restoreKey); err != nil {
		return nil, fmt.Errorf("commit Pod restore transaction: %w", err)
	}

	transactionActive = false

	return response, nil
}

func (s *Server) existingPodRestoreResponse(sandboxID, runtimeHandler string, configs []*types.ContainerConfig) (*types.RestorePodResponse, error) {
	sandbox := s.GetSandbox(sandboxID)
	if sandbox == nil || sandbox.Stopped() || s.resolvedRuntimeHandler(sandbox.RuntimeHandler()) != runtimeHandler {
		return nil, status.Error(codes.AlreadyExists, "a non-restorable pod sandbox already exists for the restore target")
	}

	containersByName := make(map[string]*oci.Container, len(configs))
	for _, container := range sandbox.Containers().List() {
		containersByName[container.Metadata().GetName()] = container
	}

	if len(containersByName) != len(configs) {
		return nil, status.Error(codes.AlreadyExists, "an incomplete or unrelated pod sandbox already exists for the restore target")
	}

	response := &types.RestorePodResponse{PodSandboxId: sandboxID}

	for _, config := range configs {
		name := config.GetMetadata().GetName()

		container := containersByName[name]
		if container == nil || container.State().Status != oci.ContainerStateCreated || !container.Restore() {
			return nil, status.Errorf(codes.AlreadyExists, "container %q for the restore target is not pending restore", name)
		}

		archive := container.RestoreArchivePath()

		info, err := os.Lstat(archive)
		if err != nil || !info.Mode().IsRegular() {
			return nil, status.Errorf(codes.AlreadyExists, "container %q has no staged restore archive", name)
		}

		response.RestoredContainers = append(response.RestoredContainers, &types.RestoredContainer{
			Name:        name,
			ContainerId: container.ID(),
		})
	}

	return response, nil
}

func copyCheckpointArchive(source, destination string) (retErr error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".pod-restore-")
	if err != nil {
		return err
	}

	temporaryName := temporary.Name()

	closed := false
	defer func() {
		if !closed {
			if err := temporary.Close(); err != nil && retErr == nil {
				retErr = err
			}
		}

		if retErr != nil {
			_ = os.Remove(temporaryName)
		}
	}()

	if err := temporary.Chmod(0o600); err != nil {
		return err
	}

	if _, err := io.Copy(temporary, input); err != nil {
		return err
	}

	if err := temporary.Sync(); err != nil {
		return err
	}

	if err := temporary.Close(); err != nil {
		return err
	}

	closed = true

	if err := os.Rename(temporaryName, destination); err != nil {
		return err
	}

	return syncCheckpointDirectory(filepath.Dir(destination))
}

func (s *Server) podRestoreTransactionDirectory() string {
	return filepath.Join(s.Store().GraphRoot(), "crio", podRestoreTransactionDir)
}

func (s *Server) podRestoreTransactionPath(key string) string {
	digest := sha256.Sum256([]byte(key))

	return filepath.Join(s.podRestoreTransactionDirectory(), hex.EncodeToString(digest[:])+".json")
}

func (s *Server) writePodRestoreTransaction(transaction *podRestoreTransaction) error {
	dir := s.podRestoreTransactionDirectory()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	return writeCheckpointJSON(dir, filepath.Base(s.podRestoreTransactionPath(transaction.Key)), transaction)
}

func (s *Server) beginPodRestoreTransaction(ctx context.Context, transaction *podRestoreTransaction) error {
	path := s.podRestoreTransactionPath(transaction.Key)
	if _, err := os.Lstat(path); err == nil {
		previous := new(podRestoreTransaction)
		if err := readCheckpointJSON(path, previous); err != nil {
			return fmt.Errorf("read previous Pod restore transaction: %w", err)
		}

		if previous.Version != podRestoreTransactionVersion || previous.Key != transaction.Key || previous.SandboxName != transaction.SandboxName {
			return errors.New("previous Pod restore transaction does not match the restore target")
		}

		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), podCheckpointCleanupTimeout)
		defer cancel()

		if err := s.rollbackPodRestore(cleanupCtx, previous); err != nil {
			return fmt.Errorf("recover previous Pod restore transaction: %w", err)
		}

		if err := s.removePodRestoreTransaction(previous.Key); err != nil {
			return fmt.Errorf("remove previous Pod restore transaction: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return s.writePodRestoreTransaction(transaction)
}

func (s *Server) removePodRestoreTransaction(key string) error {
	path := s.podRestoreTransactionPath(key)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	return syncCheckpointDirectory(filepath.Dir(path))
}

func (s *Server) recoverPodRestores(ctx context.Context) error {
	dir := s.podRestoreTransactionDirectory()

	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
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

		transaction := new(podRestoreTransaction)

		path := filepath.Join(dir, entry.Name())
		if err := readCheckpointJSON(path, transaction); err != nil {
			return err
		}

		if transaction.Version != podRestoreTransactionVersion || transaction.Key == "" || transaction.SandboxName == "" {
			return fmt.Errorf("invalid Pod restore transaction %s", path)
		}

		if path != s.podRestoreTransactionPath(transaction.Key) {
			return fmt.Errorf("pod restore transaction %s has an invalid filename", path)
		}

		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), podCheckpointCleanupTimeout)
		if err := s.rollbackPodRestore(cleanupCtx, transaction); err != nil {
			cancel()

			return err
		}

		cancel()

		if err := os.Remove(path); err != nil {
			return err
		}
	}

	return syncCheckpointDirectory(dir)
}

func (s *Server) rollbackPodRestore(ctx context.Context, transaction *podRestoreTransaction) error {
	sandboxID := transaction.SandboxID
	if sandboxID == "" {
		if resolvedID, err := s.PodIDForName(transaction.SandboxName); err == nil {
			sandboxID = resolvedID
		}
	}

	if sandboxID == "" || s.GetSandbox(sandboxID) == nil {
		return nil
	}

	if _, err := s.StopPodSandbox(ctx, &types.StopPodSandboxRequest{PodSandboxId: sandboxID}); err != nil {
		return fmt.Errorf("stop interrupted restore sandbox %q: %w", sandboxID, err)
	}

	if _, err := s.RemovePodSandbox(ctx, &types.RemovePodSandboxRequest{PodSandboxId: sandboxID}); err != nil {
		return fmt.Errorf("remove interrupted restore sandbox %q: %w", sandboxID, err)
	}

	return nil
}
