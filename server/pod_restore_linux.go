//go:build linux

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	cstorage "go.podman.io/storage"
	"go.podman.io/storage/pkg/stringid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/oci"
	pkgstorage "github.com/cri-o/cri-o/internal/storage"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

const (
	podRestoreTransactionDir     = "pod-restore-transactions"
	podRestoreTransactionVersion = 1
)

type podRestoreTransaction struct {
	Version        int                       `json:"version"`
	Key            string                    `json:"key"`
	SandboxName    string                    `json:"sandboxName"`
	SandboxID      string                    `json:"sandboxId,omitempty"`
	ContainerIDs   []string                  `json:"containerIds,omitempty"`
	Fingerprint    string                    `json:"fingerprint,omitempty"`
	RuntimeHandler string                    `json:"runtimeHandler,omitempty"`
	Response       *types.RestorePodResponse `json:"response,omitempty"`
}

func (s *Server) restorePod(
	ctx context.Context,
	request *types.RestorePodRequest,
) (_ *types.RestorePodResponse, retErr error) {
	if err := validateCheckpointInput(request.GetCheckpointPath()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid checkpoint path: %v", err)
	}

	if request.GetConfig() == nil || request.GetConfig().GetMetadata() == nil {
		return nil, status.Error(codes.InvalidArgument, "sandbox config and metadata are required")
	}

	if len(request.GetContainerConfigs()) == 0 ||
		len(request.GetContainerConfigs()) > podCheckpointMaxContainers {
		return nil, status.Error(
			codes.InvalidArgument,
			"between 1 and 1024 container configs are required",
		)
	}

	runtimeType, err := s.Runtime().RuntimeType(request.GetRuntimeHandler())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "resolve runtime handler: %v", err)
	}

	if runtimeType != libconfig.DefaultRuntimeType {
		return nil, status.Errorf(
			codes.Unimplemented,
			"Pod restore does not support runtime type %q",
			runtimeType,
		)
	}

	checkpointDirectory, err := openCheckpointDirectory(request.GetCheckpointPath())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "open checkpoint directory: %v", err)
	}
	defer checkpointDirectory.Close()

	manifest := new(podCheckpointManifest)
	if err := readCheckpointJSONAt(
		checkpointDirectory,
		podCheckpointManifestFile,
		manifest,
	); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "read Pod checkpoint manifest: %v", err)
	}

	if manifest.Version != podCheckpointFormatVersion || manifest.Runtime != "cri-o" {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"unsupported Pod checkpoint format version %d for runtime %q",
			manifest.Version,
			manifest.Runtime,
		)
	}

	restoreRuntimeHandler := s.resolvedRuntimeHandler(request.GetRuntimeHandler())
	if manifest.RuntimeHandler != restoreRuntimeHandler {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"restore runtime handler %q does not match checkpoint runtime handler %q",
			restoreRuntimeHandler,
			manifest.RuntimeHandler,
		)
	}

	checkpointSandboxConfig := new(types.PodSandboxConfig)
	if err := readCheckpointJSONAt(
		checkpointDirectory,
		podCheckpointConfigFile,
		checkpointSandboxConfig,
	); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "read checkpoint sandbox config: %v", err)
	}

	if err := validateRestoreSandboxConfig(
		checkpointSandboxConfig,
		request.GetConfig(),
	); err != nil {
		return nil, status.Errorf(
			codes.FailedPrecondition,
			"validate restore sandbox config: %v",
			err,
		)
	}

	requestedByName := make(map[string]*types.ContainerConfig, len(request.GetContainerConfigs()))
	for _, config := range request.GetContainerConfigs() {
		name := config.GetMetadata().GetName()
		if name == "" {
			return nil, status.Error(
				codes.InvalidArgument,
				"every restore container config must have a metadata name",
			)
		}

		if config.GetImage() == nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"restore container %q has no image",
				name,
			)
		}

		if _, exists := requestedByName[name]; exists {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"restore container name %q is duplicated",
				name,
			)
		}

		requestedByName[name] = config
	}

	if len(manifest.Containers) != len(requestedByName) {
		return nil, status.Error(
			codes.FailedPrecondition,
			"restore container set does not match checkpoint",
		)
	}

	archiveDirectory, err := openCheckpointDirectoryAt(
		checkpointDirectory,
		podCheckpointContainerDir,
	)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "open checkpoint archives: %v", err)
	}
	defer archiveDirectory.Close()

	archives := make(map[string]*os.File, len(manifest.Containers))
	defer func() {
		for _, file := range archives {
			file.Close()
		}
	}()

	seenIDs := make(map[string]bool, len(manifest.Containers))
	checkpointByName := make(map[string]podCheckpointManifestContainer, len(manifest.Containers))
	for _, container := range manifest.Containers {
		if container.Name == "" || container.ID == "" || container.Config == nil {
			return nil, status.Error(
				codes.InvalidArgument,
				"checkpoint manifest contains an incomplete container entry",
			)
		}

		if _, exists := checkpointByName[container.Name]; exists {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"checkpoint contains duplicate container name %q",
				container.Name,
			)
		}

		if seenIDs[container.ID] || container.Config.GetMetadata().GetName() != container.Name {
			return nil, status.Error(
				codes.InvalidArgument,
				"checkpoint manifest has duplicate IDs or inconsistent container names",
			)
		}

		seenIDs[container.ID] = true

		if container.Archive != checkpointArchiveName(container.ID) {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"checkpoint archive path for container %q is invalid",
				container.Name,
			)
		}

		file, err := openCheckpointFileAt(archiveDirectory, filepath.Base(container.Archive))
		if err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"open checkpoint archive for %q: %v",
				container.Name,
				err,
			)
		}

		archives[container.Name] = file

		digest, err := checkpointFileSHA256(file)
		if err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"hash checkpoint archive for %q: %v",
				container.Name,
				err,
			)
		}

		if digest != container.SHA256 {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"checkpoint archive checksum for container %q does not match manifest",
				container.Name,
			)
		}

		restoreConfig, exists := requestedByName[container.Name]
		if !exists {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"checkpoint container %q is missing from restore request",
				container.Name,
			)
		}

		if err := validateRestoreContainerConfig(container.Config, restoreConfig); err != nil {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"validate restore config for container %q: %v",
				container.Name,
				err,
			)
		}

		imageID, err := checkpointArchiveImageID(file, &container, manifest.RuntimeHandler)
		if err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"validate checkpoint archive for %q: %v",
				container.Name,
				err,
			)
		}

		container.ImageID = imageID

		checkpointByName[container.Name] = container
	}

	restoreKey := podRestoreKey(request.GetConfig().GetMetadata())
	if _, loaded := s.podRestoresInProgress.LoadOrStore(restoreKey, struct{}{}); loaded {
		return nil, status.Errorf(
			codes.Aborted,
			"restore for Pod %q is already in progress",
			restoreKey,
		)
	}
	defer s.podRestoresInProgress.Delete(restoreKey)

	transaction := podRestoreTransaction{
		Version:        podRestoreTransactionVersion,
		Key:            restoreKey,
		SandboxName:    makeSandboxContainerName(request.GetConfig()),
		RuntimeHandler: restoreRuntimeHandler,
	}

	fingerprint, err := podRestoreFingerprint(request, manifest, checkpointSandboxConfig)
	if err != nil {
		return nil, err
	}

	transaction.Fingerprint = fingerprint

	previous, err := s.readPodRestoreTransaction(restoreKey)
	if err == nil {
		if previous.Fingerprint != "" && previous.Fingerprint != fingerprint {
			return nil, status.Error(
				codes.AlreadyExists,
				"restore target belongs to a different checkpoint or request",
			)
		}

		if previous.Response != nil {
			if previous.RuntimeHandler != restoreRuntimeHandler {
				return nil, status.Error(
					codes.AlreadyExists,
					"restore target uses a different runtime handler",
				)
			}

			if err := syncCheckpointDirectory(s.podRestoreTransactionDirectory()); err != nil {
				return nil, err
			}

			return s.existingPodRestoreResponse(previous, request.GetContainerConfigs())
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read Pod restore transaction: %w", err)
	} else if _, lookupErr := s.PodIDForName(transaction.SandboxName); lookupErr == nil {
		return nil, status.Error(
			codes.AlreadyExists,
			"an unrelated sandbox already exists for the restore target",
		)
	}

	for _, config := range request.GetContainerConfigs() {
		imageStatus, err := s.ImageStatus(ctx, &types.ImageStatusRequest{Image: config.GetImage()})
		if err != nil {
			return nil, fmt.Errorf(
				"inspect image for container %q: %w",
				config.GetMetadata().GetName(),
				err,
			)
		}

		if imageStatus.GetImage() == nil {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"base image for container %q must be pulled before RestorePod",
				config.GetMetadata().GetName(),
			)
		}

		checkpoint := checkpointByName[config.GetMetadata().GetName()]
		if imageStatus.GetImage().GetId() != checkpoint.ImageID {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"base image for container %q does not match the checkpoint",
				config.GetMetadata().GetName(),
			)
		}
	}

	transaction.SandboxID = stringid.GenerateNonCryptoID()
	if err := s.beginPodRestoreTransaction(ctx, &transaction); err != nil {
		return nil, fmt.Errorf("write Pod restore transaction: %w", err)
	}

	transactionActive := true
	defer func() {
		if !transactionActive {
			return
		}

		cleanupCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			podCheckpointCleanupTimeout,
		)
		defer cancel()

		if err := s.rollbackPodRestore(cleanupCtx, &transaction); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("roll back Pod restore transaction: %w", err))

			return
		}

		if err := s.removePodRestoreTransaction(restoreKey); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("remove Pod restore transaction: %w", err))
		}
	}()

	sandboxResponse, err := s.runPodSandboxRequest(ctx, &types.RunPodSandboxRequest{
		Config:         request.GetConfig(),
		RuntimeHandler: request.GetRuntimeHandler(),
	}, transaction.SandboxID)
	if err != nil {
		return nil, fmt.Errorf("create restore sandbox: %w", err)
	}

	sandboxID := sandboxResponse.GetPodSandboxId()

	if sandboxID != transaction.SandboxID {
		return nil, errors.New("restored sandbox ID does not match the transaction owner")
	}

	response := &types.RestorePodResponse{PodSandboxId: sandboxID}

	for _, config := range request.GetContainerConfigs() {
		name := config.GetMetadata().GetName()

		createResponse, err := s.createContainer(ctx, &types.CreateContainerRequest{
			PodSandboxId:  sandboxID,
			Config:        config,
			SandboxConfig: request.GetConfig(),
		}, restoreKey)
		if err != nil {
			return nil, fmt.Errorf("create restored container %q: %w", name, err)
		}

		container, err := s.GetContainerFromShortID(ctx, createResponse.GetContainerId())
		if err != nil {
			return nil, fmt.Errorf("load restored container %q: %w", name, err)
		}

		if container.State().Status != oci.ContainerStateCreated {
			return nil, fmt.Errorf(
				"restored container %q is %s, expected created",
				name,
				container.State().Status,
			)
		}

		checkpointContainer := checkpointByName[name]
		if container.ImageID() == nil ||
			container.ImageID().
				IDStringForOutOfProcessConsumptionOnly() !=
				checkpointContainer.ImageID {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"created container %q has a different base image",
				name,
			)
		}

		destination := filepath.Join(container.Dir(), podRestoreArchiveFile)
		if err := copyCheckpointArchive(
			archives[name],
			destination,
			&checkpointContainer,
			manifest.RuntimeHandler,
		); err != nil {
			return nil, fmt.Errorf("stage checkpoint for container %q: %w", name, err)
		}

		container.SetRestore(true)
		container.SetRestoreArchivePath(destination)

		if err := s.ContainerStateToDisk(ctx, container); err != nil {
			return nil, fmt.Errorf("persist restore state for container %q: %w", name, err)
		}

		if err := syncCheckpointDirectory(filepath.Dir(container.StatePath())); err != nil {
			return nil, err
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

	transaction.Response = response
	if _, err := s.existingPodRestoreResponse(
		&transaction,
		request.GetContainerConfigs(),
	); err != nil {
		return nil, err
	}

	if err := s.writePodRestoreTransaction(&transaction); err != nil {
		// Rename may have published the completed receipt before directory
		// sync failed. Preserve those resources for recovery or a retry.
		published, readErr := s.readPodRestoreTransaction(restoreKey)
		if readErr != nil || published.Response != nil {
			transactionActive = false
		}

		return nil, fmt.Errorf("commit Pod restore transaction: %w", err)
	}

	transactionActive = false

	return response, nil
}

func (s *Server) existingPodRestoreResponse(
	transaction *podRestoreTransaction,
	configs []*types.ContainerConfig,
) (*types.RestorePodResponse, error) {
	sb := s.GetSandbox(transaction.SandboxID)
	if sb == nil || sb.Stopped() ||
		s.resolvedRuntimeHandler(sb.RuntimeHandler()) != transaction.RuntimeHandler {
		return nil, status.Error(
			codes.FailedPrecondition,
			"restored sandbox is no longer available",
		)
	}

	response := transaction.Response
	if response == nil || response.GetPodSandboxId() != sb.ID() ||
		len(response.GetRestoredContainers()) != len(configs) {
		return nil, status.Error(
			codes.FailedPrecondition,
			"restore transaction has an incomplete container mapping",
		)
	}

	expected := make(map[string]bool, len(configs))
	for _, config := range configs {
		expected[config.GetMetadata().GetName()] = true
	}

	for _, restored := range response.GetRestoredContainers() {
		if !expected[restored.GetName()] {
			return nil, status.Error(
				codes.FailedPrecondition,
				"restore transaction has an inconsistent container mapping",
			)
		}

		delete(expected, restored.GetName())

		container := s.GetContainer(context.Background(), restored.GetContainerId())
		if container == nil || container.Sandbox() != sb.ID() ||
			container.Metadata().GetName() != restored.GetName() ||
			container.PodRestoreKey() != transaction.Key {
			return nil, status.Errorf(
				codes.FailedPrecondition,
				"restored container %q is missing or changed",
				restored.GetName(),
			)
		}

		if container.State().Status == oci.ContainerStateCreated {
			if !container.Restore() {
				return nil, status.Errorf(
					codes.FailedPrecondition,
					"container %q has no pending restore",
					restored.GetName(),
				)
			}

			archive, err := openCheckpointFile(container.RestoreArchivePath())
			if err != nil {
				return nil, status.Errorf(
					codes.FailedPrecondition,
					"container %q has no staged restore archive: %v",
					restored.GetName(),
					err,
				)
			}

			archive.Close()
		}
	}

	return proto.CloneOf(response), nil
}

func podRestoreKey(metadata *types.PodSandboxMetadata) string {
	return fmt.Sprintf(
		"%s/%s/%s/%d",
		metadata.GetNamespace(),
		metadata.GetName(),
		metadata.GetUid(),
		metadata.GetAttempt(),
	)
}

func podRestoreFingerprint(
	request *types.RestorePodRequest,
	manifest *podCheckpointManifest,
	config *types.PodSandboxConfig,
) (string, error) {
	canonical := proto.CloneOf(request)
	canonical.CheckpointPath = filepath.Clean(canonical.GetCheckpointPath())
	slices.SortFunc(canonical.GetContainerConfigs(), func(a, b *types.ContainerConfig) int {
		return strings.Compare(a.GetMetadata().GetName(), b.GetMetadata().GetName())
	})

	data, err := json.Marshal(struct {
		Request       *types.RestorePodRequest `json:"request"`
		Manifest      *podCheckpointManifest   `json:"manifest"`
		SandboxConfig *types.PodSandboxConfig  `json:"sandboxConfig"`
	}{canonical, manifest, config})
	if err != nil {
		return "", fmt.Errorf("encode Pod restore identity: %w", err)
	}

	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:]), nil
}

func (s *Server) readPodRestoreTransaction(key string) (*podRestoreTransaction, error) {
	transaction := new(podRestoreTransaction)
	if err := readCheckpointJSON(s.podRestoreTransactionPath(key), transaction); err != nil {
		return nil, err
	}

	if transaction.Version != podRestoreTransactionVersion || transaction.Key != key ||
		transaction.SandboxName == "" {
		return nil, errors.New("invalid Pod restore transaction identity")
	}

	if transaction.Response != nil &&
		(transaction.Fingerprint == "" || transaction.SandboxID == "" || transaction.Response.GetPodSandboxId() != transaction.SandboxID) {
		return nil, errors.New("invalid completed Pod restore transaction")
	}

	return transaction, nil
}

func (s *Server) requirePodRestoreCommitted(container *oci.Container) error {
	key := container.PodRestoreKey()
	if key == "" {
		return nil
	}

	if _, active := s.podRestoresInProgress.Load(key); active {
		return status.Error(
			codes.FailedPrecondition,
			"Pod restore preparation is still in progress",
		)
	}

	transaction, err := s.readPodRestoreTransaction(key)
	if err != nil {
		return fmt.Errorf("read container Pod restore transaction: %w", err)
	}

	if transaction.Response != nil && transaction.SandboxID == container.Sandbox() {
		for _, restored := range transaction.Response.GetRestoredContainers() {
			if restored.GetContainerId() == container.ID() &&
				restored.GetName() == container.Metadata().GetName() {
				return syncCheckpointDirectory(s.podRestoreTransactionDirectory())
			}
		}
	}

	return status.Error(codes.FailedPrecondition, "Pod restore preparation has not committed")
}

// Keep completed receipts until sandbox removal so lost responses can be
// retried after daemon restart or after individual containers have started.
func (s *Server) removeCompletedPodRestore(sb *sandbox.Sandbox) error {
	key := podRestoreKey(sb.Metadata())

	transaction, err := s.readPodRestoreTransaction(key)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return err
	}

	if transaction.Response == nil || transaction.SandboxID != sb.ID() {
		return nil
	}

	return s.removePodRestoreTransaction(key)
}

func copyCheckpointArchive(
	input *os.File,
	destination string,
	checkpoint *podCheckpointManifestContainer,
	runtimeHandler string,
) (retErr error) {
	info, err := input.Stat()
	if err != nil {
		return err
	}

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

	hash := sha256.New()
	if _, err := io.Copy(
		io.MultiWriter(temporary, hash),
		io.NewSectionReader(input, 0, info.Size()),
	); err != nil {
		return err
	}

	if hex.EncodeToString(hash.Sum(nil)) != checkpoint.SHA256 {
		return errors.New("checkpoint archive changed while staging restore")
	}

	// Check the copied bytes, too: the source inode can change between its
	// initial hash and metadata read. Only this private copy is restored.
	if _, err := checkpointArchiveImageID(temporary, checkpoint, runtimeHandler); err != nil {
		return fmt.Errorf("validate staged checkpoint identity: %w", err)
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

	return writeCheckpointJSON(
		dir,
		filepath.Base(s.podRestoreTransactionPath(transaction.Key)),
		transaction,
	)
}

func (s *Server) beginPodRestoreTransaction(
	ctx context.Context,
	transaction *podRestoreTransaction,
) error {
	path := s.podRestoreTransactionPath(transaction.Key)
	if _, err := os.Lstat(path); err == nil {
		previous := new(podRestoreTransaction)
		if err := readCheckpointJSON(path, previous); err != nil {
			return fmt.Errorf("read previous Pod restore transaction: %w", err)
		}

		if previous.Version != podRestoreTransactionVersion || previous.Key != transaction.Key ||
			previous.SandboxName != transaction.SandboxName {
			return errors.New("previous Pod restore transaction does not match the restore target")
		}

		if previous.Response != nil {
			return status.Error(
				codes.AlreadyExists,
				"a committed restore already exists for the target",
			)
		}

		cleanupCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			podCheckpointCleanupTimeout,
		)
		defer cancel()

		if err := s.rollbackPodRestore(cleanupCtx, previous); err != nil {
			return fmt.Errorf("recover previous Pod restore transaction: %w", err)
		}

		if err := s.removePodRestoreTransaction(previous.Key); err != nil {
			return fmt.Errorf("remove previous Pod restore transaction: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return s.writePodRestoreTransaction(transaction)
}

func (s *Server) removePodRestoreTransaction(key string) error {
	path := s.podRestoreTransactionPath(key)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return syncCheckpointDirectory(filepath.Dir(path))
}

func (s *Server) recoverPodRestores(ctx context.Context) error {
	dir := s.podRestoreTransactionDirectory()

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

		transaction := new(podRestoreTransaction)

		path := filepath.Join(dir, entry.Name())
		if err := readCheckpointJSON(path, transaction); err != nil {
			return err
		}

		if transaction.Version != podRestoreTransactionVersion || transaction.Key == "" ||
			transaction.SandboxName == "" {
			return fmt.Errorf("invalid Pod restore transaction %s", path)
		}

		if path != s.podRestoreTransactionPath(transaction.Key) {
			return fmt.Errorf("pod restore transaction %s has an invalid filename", path)
		}

		if transaction.Response != nil {
			if _, err := s.readPodRestoreTransaction(transaction.Key); err != nil {
				return err
			}

			continue
		}

		cleanupCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx),
			podCheckpointCleanupTimeout,
		)
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
		// Older markers can predate recording the returned ID. A name alone
		// does not establish ownership of an existing sandbox.
		if _, err := s.PodIDForName(transaction.SandboxName); err == nil {
			return errors.New("interrupted restore has no recorded sandbox owner")
		}

		if _, err := s.Store().Container(transaction.SandboxName); err == nil {
			return errors.New("interrupted restore has unclaimed sandbox storage")
		} else if !errors.Is(err, cstorage.ErrContainerUnknown) {
			return err
		}

		return nil
	}

	if s.GetSandbox(sandboxID) == nil {
		if _, err := s.Store().Container(sandboxID); errors.Is(err, cstorage.ErrContainerUnknown) {
			return nil
		} else if err != nil {
			return err
		}

		return fmt.Errorf("restore sandbox %q has not been loaded", sandboxID)
	}

	stored, err := s.Store().Containers()
	if err != nil {
		return err
	}

	for i := range stored {
		container := &stored[i]
		if container.ID == sandboxID {
			continue
		}

		data, err := s.Store().Metadata(container.ID)
		if err != nil {
			return err
		}

		var metadata pkgstorage.RuntimeContainerMetadata
		if err := json.Unmarshal([]byte(data), &metadata); err != nil {
			return err
		}

		if metadata.PodID == sandboxID && s.GetContainer(ctx, container.ID) == nil {
			return fmt.Errorf("restore container %q has not been loaded", container.ID)
		}
	}

	if _, err := s.StopPodSandbox(
		ctx,
		&types.StopPodSandboxRequest{PodSandboxId: sandboxID},
	); err != nil {
		return fmt.Errorf("stop interrupted restore sandbox %q: %w", sandboxID, err)
	}

	if _, err := s.RemovePodSandbox(
		ctx,
		&types.RemovePodSandboxRequest{PodSandboxId: sandboxID},
	); err != nil {
		return fmt.Errorf("remove interrupted restore sandbox %q: %w", sandboxID, err)
	}

	return nil
}
