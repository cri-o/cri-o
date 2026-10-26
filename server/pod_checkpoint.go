package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

const (
	podCheckpointFormatVersion = 1
	podCheckpointMaxContainers = 1024
	podCheckpointManifestFile  = "checkpoint-manifest.json"
	podCheckpointConfigFile    = "pod-config.json"
	podCheckpointContainerDir  = "containers"
	podRestoreArchiveFile      = "pod-restore-checkpoint.tar"
)

type podCheckpointManifest struct {
	Version        int                              `json:"version"`
	Runtime        string                           `json:"runtime"`
	RuntimeHandler string                           `json:"runtimeHandler"`
	SandboxID      string                           `json:"sandboxId"`
	Containers     []podCheckpointManifestContainer `json:"containers"`
}

type podCheckpointManifestContainer struct {
	Name    string                 `json:"name"`
	ID      string                 `json:"id"`
	Archive string                 `json:"archive"`
	SHA256  string                 `json:"sha256"`
	Config  *types.ContainerConfig `json:"config"`
	ImageID string                 `json:"imageId,omitempty"`
}

// CheckpointPod checkpoints a consistent set of containers in a pod sandbox.
func (s *Server) CheckpointPod(
	ctx context.Context,
	request *types.CheckpointPodRequest,
) (*types.CheckpointPodResponse, error) {
	if !s.config.CheckpointContainerEnabled() {
		return nil, status.Error(codes.Unimplemented, "checkpoint support is not enabled")
	}

	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}

	if _, ok := ctx.Deadline(); !ok {
		return nil, status.Error(codes.InvalidArgument, "CheckpointPod requires a finite deadline")
	}

	if len(request.GetOptions()) != 0 {
		return nil, status.Error(codes.InvalidArgument, "CheckpointPod options are not supported")
	}

	return s.checkpointPod(ctx, request)
}

// RestorePod creates a sandbox and containers prepared to restore from a Pod checkpoint.
func (s *Server) RestorePod(
	ctx context.Context,
	request *types.RestorePodRequest,
) (*types.RestorePodResponse, error) {
	if !s.config.RestoreContainerEnabled() {
		return nil, status.Error(codes.Unimplemented, "restore support is not enabled")
	}

	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}

	if _, ok := ctx.Deadline(); !ok {
		return nil, status.Error(codes.InvalidArgument, "RestorePod requires a finite deadline")
	}

	if len(request.GetOptions()) != 0 {
		return nil, status.Error(codes.InvalidArgument, "RestorePod options are not supported")
	}

	return s.restorePod(ctx, request)
}

func (s *Server) reservePodCheckpoint(
	sandboxID, outputPath string,
	containerIDs []string,
) (func(), error) {
	canonicalOutput, err := filepath.EvalSymlinks(outputPath)
	if err != nil {
		return nil, fmt.Errorf("resolve checkpoint output path: %w", err)
	}

	if _, loaded := s.podCheckpointOutputs.LoadOrStore(canonicalOutput, struct{}{}); loaded {
		return nil, fmt.Errorf("checkpoint output path %q is already in use", outputPath)
	}

	if _, loaded := s.podCheckpointsInProgress.LoadOrStore(sandboxID, struct{}{}); loaded {
		s.podCheckpointOutputs.Delete(canonicalOutput)

		return nil, fmt.Errorf("checkpoint for pod sandbox %q is already in progress", sandboxID)
	}

	releaseContainers, err := s.reserveContainerCheckpoints(containerIDs)
	if err != nil {
		s.podCheckpointsInProgress.Delete(sandboxID)
		s.podCheckpointOutputs.Delete(canonicalOutput)

		return nil, err
	}

	return func() {
		releaseContainers()
		s.podCheckpointsInProgress.Delete(sandboxID)
		s.podCheckpointOutputs.Delete(canonicalOutput)
	}, nil
}

func (s *Server) reserveContainerCheckpoints(containerIDs []string) (func(), error) {
	ids := slices.Clone(containerIDs)
	sort.Strings(ids)

	reserved := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, loaded := s.containerCheckpointsInProgress.LoadOrStore(id, struct{}{}); loaded {
			for _, acquired := range reserved {
				s.containerCheckpointsInProgress.Delete(acquired)
			}

			return nil, fmt.Errorf(
				"checkpoint operation for container %q is already in progress",
				id,
			)
		}

		reserved = append(reserved, id)
	}

	return func() {
		for _, id := range reserved {
			s.containerCheckpointsInProgress.Delete(id)
		}
	}, nil
}

func validateEmptyCheckpointOutput(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("checkpoint output path must be absolute")
	}

	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect checkpoint output path: %w", err)
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("checkpoint output path must be a real directory")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("read checkpoint output directory: %w", err)
	}

	if len(entries) != 0 {
		return errors.New("checkpoint output directory must be empty")
	}

	return nil
}

func validateCheckpointInput(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("checkpoint path must be absolute")
	}

	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect checkpoint path: %w", err)
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("checkpoint path must be a real directory")
	}

	return nil
}

func checkpointArchiveName(containerID string) string {
	digest := sha256.Sum256([]byte(containerID))

	return filepath.Join(podCheckpointContainerDir, hex.EncodeToString(digest[:])+".tar")
}

func fileSHA256(path string) (string, error) {
	file, err := openCheckpointFile(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	return checkpointFileSHA256(file)
}

func checkpointFileSHA256(file *os.File) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, io.NewSectionReader(file, 0, info.Size())); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateRestoreSandboxConfig(checkpoint, restore *types.PodSandboxConfig) error {
	// The destination Pod can have a different name and default hostname.

	checkpointSecurity := checkpoint.GetLinux().GetSecurityContext()
	if checkpointSecurity == nil {
		checkpointSecurity = new(types.LinuxSandboxSecurityContext)
	}

	restoreSecurity := restore.GetLinux().GetSecurityContext()
	if restoreSecurity == nil {
		restoreSecurity = new(types.LinuxSandboxSecurityContext)
	}

	if !proto.Equal(checkpointSecurity, restoreSecurity) {
		return errors.New("restore sandbox security context does not match checkpoint")
	}

	if !maps.Equal(checkpoint.GetLinux().GetSysctls(), restore.GetLinux().GetSysctls()) {
		return errors.New("restore sandbox sysctls do not match checkpoint")
	}

	if len(checkpoint.GetPortMappings()) != len(restore.GetPortMappings()) {
		return errors.New("restore sandbox port mappings do not match checkpoint")
	}

	for index := range checkpoint.GetPortMappings() {
		if !proto.Equal(checkpoint.GetPortMappings()[index], restore.GetPortMappings()[index]) {
			return errors.New("restore sandbox port mappings do not match checkpoint")
		}
	}

	return nil
}

func validateRestoreContainerConfig(checkpoint, restore *types.ContainerConfig) error {
	// Image identity is checked against the resolved image ID. Destination
	// argv and Downward API values describe the new Pod; CRIU resumes the
	// saved process with its original arguments and environment.

	if checkpoint.GetWorkingDir() != restore.GetWorkingDir() {
		return errors.New("restore working directory does not match checkpoint")
	}

	if checkpoint.GetTty() != restore.GetTty() {
		return errors.New("restore TTY setting does not match checkpoint")
	}

	checkpointSecurity := checkpoint.GetLinux().GetSecurityContext()
	if checkpointSecurity == nil {
		checkpointSecurity = new(types.LinuxContainerSecurityContext)
	}

	restoreSecurity := restore.GetLinux().GetSecurityContext()
	if restoreSecurity == nil {
		restoreSecurity = new(types.LinuxContainerSecurityContext)
	}

	if !proto.Equal(checkpointSecurity, restoreSecurity) {
		return errors.New("restore process security context does not match checkpoint")
	}

	return nil
}
