//go:build linux

/*
Copyright The CRI-O Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	cstorage "go.podman.io/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/podoperation"
	pkgstorage "github.com/cri-o/cri-o/internal/storage"
)

// StartContainer keeps restored task creation under its preparation owner's fence.
func (s *Server) startContainerWithPodOperation(
	ctx context.Context,
	r *types.StartContainerRequest,
) (*types.StartContainerResponse, error) {
	container, err := s.GetContainerFromShortID(ctx, r.GetContainerId())
	if err != nil || container.PodRestoreKey() == "" {
		return s.startContainer(ctx, r)
	}

	transaction, err := s.readPodRestoreTransaction(container.PodRestoreKey())
	if err != nil {
		return nil, fmt.Errorf("read container Pod restore transaction: %w", err)
	}

	if transaction.OperationID == "" {
		return s.startContainer(ctx, r)
	}

	state := container.State()
	if state.Status != oci.ContainerStateCreated && (!container.Restore() || state.Status != "") {
		return nil, fmt.Errorf(
			"container %s is not in created state: %s",
			container.ID(),
			state.Status,
		)
	}

	m, err := s.podOperationManager()
	if err != nil {
		return nil, err
	}

	type result struct {
		response *types.StartContainerResponse
		err      error
	}

	returned := make(chan result, 1)

	go func() {
		admitted := false

		var cleanupOnce sync.Once

		startCleanup := func() {
			cleanupOnce.Do(func() {
				go s.settleInterruptedPodOperation(
					types.PodOperationType_POD_RESTORE,
					transaction.OperationID,
				)
			})
		}

		var response *types.StartContainerResponse

		err := m.AttachResource(
			ctx,
			int32(types.PodOperationType_POD_RESTORE),
			transaction.OperationID,
			container.ID(),
			func(owner context.Context) error {
				admitted = true

				finished := make(chan struct{})
				defer close(finished)

				go func() {
					select {
					case <-ctx.Done():
						select {
						case <-finished:
							return
						default:
						}

						startCleanup()
					case <-finished:
					}
				}()

				var err error

				response, err = s.startContainer(owner, r)

				return err
			},
		)
		if admitted && err != nil {
			startCleanup()
		}

		returned <- result{response: response, err: err}
	}()

	select {
	case result := <-returned:
		return result.response, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// cachedPodOperationManager returns the manager stored under "manager". The
// same map also holds cleanup reservations, so check the stored type.
func cachedPodOperationManager(value any) (*podoperation.Manager, error) {
	m, ok := value.(*podoperation.Manager)
	if !ok {
		return nil, fmt.Errorf("unexpected Pod operation manager type %T", value)
	}

	return m, nil
}

func (s *Server) podOperationManager() (*podoperation.Manager, error) {
	if value, ok := s.podOperations.Load("manager"); ok {
		return cachedPodOperationManager(value)
	}

	root := s.podOperationRoot
	if root == "" {
		root = s.Store().GraphRoot()
	}

	if !filepath.IsAbs(root) {
		return nil, errors.New("storage graph root must be absolute for Pod operation cancellation")
	}

	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return nil, err
	}

	m, err := podoperation.New(
		filepath.Join(root, "crio", "pod-operations"),
		strings.TrimSpace(string(boot)),
	)
	if err != nil {
		return nil, err
	}

	actual, _ := s.podOperations.LoadOrStore("manager", m)

	return cachedPodOperationManager(actual)
}

func (s *Server) runPodOperation(
	ctx context.Context,
	kind types.PodOperationType,
	id string,
	request proto.Message,
	fn func(context.Context) (proto.Message, error),
) ([]byte, error) {
	m, err := s.podOperationManager()
	if err != nil {
		return nil, err
	}

	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(request)
	if err != nil {
		return nil, err
	}

	fingerprint := sha256.Sum256(payload)
	target := ""

	if restore, ok := request.(*types.RestorePodRequest); ok {
		metadata := restore.GetConfig().GetMetadata()
		if metadata.GetName() == "" || metadata.GetNamespace() == "" || metadata.GetUid() == "" {
			return nil, status.Error(
				codes.InvalidArgument,
				"restore requires Pod name, namespace and UID",
			)
		}

		identity, err := json.Marshal(
			[]any{
				metadata.GetNamespace(),
				metadata.GetName(),
				metadata.GetUid(),
				metadata.GetAttempt(),
			},
		)
		if err != nil {
			return nil, err
		}

		digest := sha256.Sum256(identity)
		target = "restore:" + hex.EncodeToString(digest[:])
	}

	type result struct {
		response []byte
		err      error
	}

	returned := make(chan result, 1)

	go func() {
		admitted := false

		var cleanupOnce sync.Once

		startCleanup := func() { cleanupOnce.Do(func() { go s.settleInterruptedPodOperation(kind, id) }) }

		response, err := m.RunTarget(
			ctx,
			int32(kind),
			id,
			hex.EncodeToString(fingerprint[:]),
			target,
			func(owner context.Context) ([]byte, error) {
				admitted = true

				finished := make(chan struct{})
				defer close(finished)

				go func() {
					select {
					case <-ctx.Done():
						select {
						case <-finished:
							return
						default:
						}

						startCleanup()
					case <-finished:
					}
				}()

				response, err := fn(owner)
				if err != nil {
					return nil, err
				}

				return proto.Marshal(response)
			},
		)
		if admitted && err != nil {
			startCleanup()
		}

		if errors.Is(err, podoperation.ErrExists) {
			err = status.Error(codes.AlreadyExists, err.Error())
		}

		returned <- result{response: response, err: err}
	}()

	select {
	case result := <-returned:
		return result.response, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Server) settleInterruptedPodOperation(kind types.PodOperationType, id string) {
	key := fmt.Sprintf("cleanup:%d:%s", kind, id)
	if _, exists := s.podOperations.LoadOrStore(key, struct{}{}); exists {
		return
	}
	defer s.podOperations.Delete(key)

	owner, stop := s.podOperationCleanupContext(context.Background(), 0)
	defer stop()

	previous := ""
	//nolint:errcheck // each failed attempt is logged; the loop ends only on success or daemon stop
	_ = retryPodOperationCleanup(owner, time.Second, func(owner context.Context) error {
		ctx, cancel := context.WithTimeout(owner, podCheckpointCleanupTimeout)
		defer cancel()

		_, err := s.cancelPodOperation(
			ctx,
			&types.CancelPodOperationRequest{OperationId: id, OperationType: kind},
		)
		if err != nil && err.Error() != previous {
			log.Warnf(ctx, "Pod operation %q cleanup remains pending: %v", id, err)
			previous = err.Error()
		}

		if s.monitorsChan == nil {
			stop()
		}

		return err
	})
}

func retryPodOperationCleanup(
	ctx context.Context,
	delay time.Duration,
	attempt func(context.Context) error,
) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := attempt(ctx)
		if err == nil {
			return nil
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()

			return errors.Join(err, ctx.Err())
		case <-timer.C:
		}

		delay = min(2*delay, time.Minute)
	}
}

// Cleanup outlives an RPC waiter, but never the daemon's monitor lifecycle.
func (s *Server) podOperationCleanupContext(
	parent context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.WithoutCancel(parent), timeout)
	} else {
		ctx, cancel = context.WithCancel(context.WithoutCancel(parent))
	}

	if s.monitorsChan != nil {
		go func() {
			select {
			case <-s.monitorsChan:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	return ctx, cancel
}

// CancelPodOperation acknowledges only a durable cancellation fence and
// completed cleanup. A failed lower-layer request retains uncertainty.
func (s *Server) CancelPodOperation(
	ctx context.Context,
	request *types.CancelPodOperationRequest,
) (*types.CancelPodOperationResponse, error) {
	// Without the Pod RPCs, there is no caller-identified operation to cancel.
	// Rejecting it also avoids creating durable fences for arbitrary IDs.
	if !podOperationCancellationSupported() {
		return nil, status.Error(codes.Unimplemented, "Pod operation cancellation is not supported")
	}

	return s.cancelPodOperation(ctx, request)
}

func (s *Server) cancelPodOperation(
	ctx context.Context,
	request *types.CancelPodOperationRequest,
) (*types.CancelPodOperationResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is nil")
	}

	if _, ok := ctx.Deadline(); !ok {
		return nil, status.Error(
			codes.InvalidArgument,
			"CancelPodOperation requires a finite deadline",
		)
	}

	if request.GetOperationId() == "" {
		return nil, status.Error(
			codes.InvalidArgument,
			"CancelPodOperation requires an operation ID",
		)
	}

	if request.GetOperationType() != types.PodOperationType_POD_CHECKPOINT &&
		request.GetOperationType() != types.PodOperationType_POD_RESTORE {
		return nil, status.Error(codes.InvalidArgument, "unsupported Pod operation type")
	}

	m, err := s.podOperationManager()
	if err != nil {
		return nil, err
	}

	err = m.Settle(
		ctx,
		int32(request.GetOperationType()),
		request.GetOperationId(),
		s.settlePodOperationBackend,
		func(ctx context.Context, record *podoperation.Record) error {
			if record.Fingerprint == "" {
				return nil
			}

			cleanupCtx, cancel := s.podOperationCleanupContext(ctx, podCheckpointCleanupTimeout)
			defer cancel()

			switch request.GetOperationType() {
			case types.PodOperationType_POD_CHECKPOINT:
				if record.ResourceID == "" {
					if !record.MayHaveEffects {
						return nil
					}

					return status.Error(
						codes.FailedPrecondition,
						"checkpoint operation has no recorded sandbox identity",
					)
				}

				return s.recoverPodCheckpointForSandbox(cleanupCtx, record.ResourceID)
			case types.PodOperationType_POD_RESTORE:
				if record.ResourceID == "" {
					if !record.MayHaveEffects {
						return nil
					}

					return status.Error(
						codes.FailedPrecondition,
						"restore operation has no recorded sandbox identity",
					)
				}

				if err := s.rollbackPodRestore(
					cleanupCtx,
					&podRestoreTransaction{OperationID: record.ID, SandboxID: record.ResourceID},
				); err != nil {
					return err
				}

				if err := s.verifyPodRestoreRemoved(record.ResourceID); err != nil {
					return err
				}

				s.ReleasePodNameForID(record.ResourceID)

				if record.TransactionKey != "" {
					transaction, err := s.readPodRestoreTransaction(record.TransactionKey)
					if errors.Is(err, os.ErrNotExist) {
						return nil
					}

					if err != nil {
						return err
					}

					if transaction.OperationID != record.ID ||
						transaction.SandboxID != record.ResourceID {
						return errors.New("restore receipt belongs to another operation")
					}

					return s.removePodRestoreTransaction(record.TransactionKey)
				}

				return nil
			default:
				return status.Error(codes.InvalidArgument, "unsupported Pod operation type")
			}
		},
	)
	if err != nil {
		if errors.Is(err, podoperation.ErrUncertain) {
			return nil, status.Error(codes.FailedPrecondition, err.Error())
		}

		return nil, err
	}

	return &types.CancelPodOperationResponse{}, nil
}

func (s *Server) settlePodOperationBackend(ctx context.Context, record *podoperation.Record) error {
	if record.Kind != int32(types.PodOperationType_POD_RESTORE) ||
		len(record.BackendResources) == 0 {
		return podoperation.ErrUncertain
	}

	for _, id := range record.BackendResources {
		if slices.Contains(record.BackendSettled, id) ||
			slices.Contains(record.RemovedResources, id) {
			continue
		}

		container := s.GetContainer(ctx, id)
		if container == nil {
			return fmt.Errorf("restore backend container %q has not been recovered", id)
		}

		if err := s.Runtime().SettlePodRestore(ctx, container, record.ID); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) settlePodRestoreBeforeRemoval(ctx context.Context, operationID string) error {
	if operationID == "" ||
		podoperation.IsCleanupFor(ctx, int32(types.PodOperationType_POD_RESTORE), operationID) {
		return nil
	}

	owner, cancel := s.podOperationCleanupContext(ctx, podCheckpointCleanupTimeout)
	defer cancel()

	_, err := s.cancelPodOperation(
		owner,
		&types.CancelPodOperationRequest{
			OperationId:   operationID,
			OperationType: types.PodOperationType_POD_RESTORE,
		},
	)
	if err != nil {
		go s.settleInterruptedPodOperation(types.PodOperationType_POD_RESTORE, operationID)
	}

	return err
}

func (s *Server) podRestoreOperationForSandbox(sandboxID string) (string, error) {
	if value, ok := s.podOperations.Load("manager"); ok {
		m, err := cachedPodOperationManager(value)
		if err != nil {
			return "", err
		}

		return m.OperationForResource(int32(types.PodOperationType_POD_RESTORE), sandboxID)
	}

	if s.ContainerServer == nil || s.Store() == nil {
		return "", nil
	}

	root := s.podOperationRoot
	if root == "" {
		root = s.Store().GraphRoot()
	}

	if root == "" {
		return "", nil
	}

	if _, err := os.Stat(filepath.Join(root, "crio", "pod-operations")); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}

	m, err := s.podOperationManager()
	if err != nil {
		return "", err
	}

	return m.OperationForResource(int32(types.PodOperationType_POD_RESTORE), sandboxID)
}

func (s *Server) removePodRestoreContainer(
	ctx context.Context,
	operationID, containerID string,
	remove func(context.Context) error,
) error {
	if operationID == "" ||
		podoperation.IsCleanupFor(ctx, int32(types.PodOperationType_POD_RESTORE), operationID) {
		return remove(ctx)
	}

	m, err := s.podOperationManager()
	if err != nil {
		return err
	}

	return m.RemoveResource(
		ctx,
		int32(types.PodOperationType_POD_RESTORE),
		operationID,
		containerID,
		func(ctx context.Context, record *podoperation.Record) error {
			container := s.GetContainer(ctx, containerID)
			if container == nil {
				return fmt.Errorf(
					"restore backend container %q has not been recovered",
					containerID,
				)
			}

			return s.Runtime().SettlePodRestore(ctx, container, record.ID)
		},
		remove,
	)
}

func (s *Server) podOperationRecoveryAllowed(
	ctx context.Context,
	kind types.PodOperationType,
	id string,
) error {
	if id == "" {
		if podoperation.ID(ctx) != "" {
			return podoperation.ErrUncertain
		}

		return nil
	}

	if podoperation.ID(ctx) != id {
		return podoperation.ErrUncertain
	}

	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return err
	}

	root := s.Store().GraphRoot()
	if !filepath.IsAbs(root) {
		return errors.New("storage graph root must be absolute for Pod operation recovery")
	}

	return podoperation.CanRecover(
		ctx,
		filepath.Join(root, "crio", "pod-operations"),
		strings.TrimSpace(string(boot)),
		int32(kind),
		id,
	)
}

func (s *Server) verifyPodRestoreRemoved(sandboxID string) error {
	if _, err := s.Store().Container(sandboxID); !errors.Is(err, cstorage.ErrContainerUnknown) {
		if err != nil {
			return err
		}

		return fmt.Errorf("restore sandbox storage %q remains", sandboxID)
	}

	stored, err := s.Store().Containers()
	if err != nil {
		return err
	}

	for i := range stored {
		data, err := s.Store().Metadata(stored[i].ID)
		if err != nil {
			return err
		}

		var metadata pkgstorage.RuntimeContainerMetadata
		if err := json.Unmarshal([]byte(data), &metadata); err != nil {
			return err
		}

		if metadata.PodID == sandboxID {
			return fmt.Errorf("restore container storage %q remains", stored[i].ID)
		}
	}

	return nil
}
