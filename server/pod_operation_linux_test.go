//go:build test && linux

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
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/require"
	cstorage "go.podman.io/storage"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/lib/sandbox"
	"github.com/cri-o/cri-o/internal/memorystore"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/internal/podoperation"
	containermock "github.com/cri-o/cri-o/test/mocks/containerstorage"
)

func TestCancelPodOperationFencesLateCheckpoint(t *testing.T) {
	s := podRestoreTestServer(t)
	s.config.SetCheckpointRestore(true)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	request := &types.CancelPodOperationRequest{
		OperationId:   "checkpoint-operation",
		OperationType: types.PodOperationType_POD_CHECKPOINT,
	}
	response, err := s.cancelPodOperation(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, response)

	reopened := &Server{ContainerServer: s.ContainerServer, config: s.config}
	late := &types.CheckpointPodRequest{
		OperationId:  request.GetOperationId(),
		PodSandboxId: "late-sandbox",
	}
	_, err = reopened.runPodOperation(
		ctx,
		types.PodOperationType_POD_CHECKPOINT,
		late.GetOperationId(),
		late,
		func(context.Context) (proto.Message, error) {
			t.Fatal("late checkpoint reached backend")

			return nil, nil
		},
	)
	require.Equal(t, codes.AlreadyExists, status.Code(err))
	response, err = reopened.cancelPodOperation(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, response)
}

func TestCancelPodOperationRequiresPodOperationSupport(t *testing.T) {
	s := podRestoreTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	_, err := s.CancelPodOperation(
		ctx,
		&types.CancelPodOperationRequest{
			OperationId:   "operation",
			OperationType: types.PodOperationType_POD_RESTORE,
		},
	)
	require.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = os.Stat(filepath.Join(s.Store().GraphRoot(), "crio", "pod-operations"))
	require.ErrorIs(
		t,
		err,
		os.ErrNotExist,
		"a rejected cancellation must not create operation state",
	)
}

func TestCancelPodOperationRejectsUnboundedAndInvalidRequests(t *testing.T) {
	s := podRestoreTestServer(t)
	_, err := s.cancelPodOperation(context.Background(), nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = s.cancelPodOperation(
		context.Background(),
		&types.CancelPodOperationRequest{
			OperationId:   "operation",
			OperationType: types.PodOperationType_POD_CHECKPOINT,
		},
	)
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = s.cancelPodOperation(ctx, &types.CancelPodOperationRequest{OperationId: "operation"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = s.cancelPodOperation(
		ctx,
		&types.CancelPodOperationRequest{OperationType: types.PodOperationType_POD_RESTORE},
	)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestUncertainPodOperationBlocksRecovery(t *testing.T) {
	for _, kind := range []types.PodOperationType{types.PodOperationType_POD_CHECKPOINT, types.PodOperationType_POD_RESTORE} {
		t.Run(kind.String(), func(t *testing.T) {
			s := podRestoreTestServer(t)
			m, err := s.podOperationManager()
			require.NoError(t, err)
			_, err = m.Run(
				context.Background(),
				int32(kind),
				"operation",
				"request",
				func(ctx context.Context) ([]byte, error) {
					return nil, podoperation.Join(
						ctx,
						func(context.Context) error { return errors.New("child transport lost") },
					)
				},
			)
			require.ErrorContains(t, err, "child transport lost")

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			_, err = s.cancelPodOperation(
				ctx,
				&types.CancelPodOperationRequest{OperationId: "operation", OperationType: kind},
			)
			require.Equal(t, codes.FailedPrecondition, status.Code(err))

			if kind == types.PodOperationType_POD_CHECKPOINT {
				err = s.recoverPodCheckpointMarker(
					ctx,
					&podCheckpointRecoveryMarker{OperationID: "operation", SandboxID: "sandbox"},
				)
			} else {
				err = s.rollbackPodRestore(
					ctx,
					&podRestoreTransaction{OperationID: "operation", SandboxID: "sandbox"},
				)
			}

			require.ErrorIs(t, err, podoperation.ErrUncertain)
		})
	}
}

func TestCancelFailedRestorePreparationAcknowledgesNoEffects(t *testing.T) {
	s := podRestoreTestServer(t)
	s.config.SetCheckpointRestore(true)

	request := &types.RestorePodRequest{OperationId: "failed-preparation"}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := s.RestorePod(ctx, request)
	require.Error(t, err)
	response, err := s.cancelPodOperation(
		ctx,
		&types.CancelPodOperationRequest{
			OperationId:   request.GetOperationId(),
			OperationType: types.PodOperationType_POD_RESTORE,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, response)
}

func TestCancelPodOperationWaitsForCheckpointWriter(t *testing.T) {
	s := podRestoreTestServer(t)
	request := &types.CheckpointPodRequest{
		OperationId:  "checkpoint-writer",
		PodSandboxId: "sandbox",
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	returned := make(chan error, 1)
	output := filepath.Join(t.TempDir(), "writer-output")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, err := s.runPodOperation(
			ctx,
			types.PodOperationType_POD_CHECKPOINT,
			request.GetOperationId(),
			request,
			func(ctx context.Context) (proto.Message, error) {
				if err := podoperation.SetResource(ctx, request.GetPodSandboxId()); err != nil {
					return nil, err
				}

				err := podoperation.Join(ctx, func(joined context.Context) error {
					close(entered)
					<-release

					return os.WriteFile(output, []byte("finished"), 0o600)
				})

				return &types.CheckpointPodResponse{}, err
			},
		)
		returned <- err
	}()

	<-entered
	cancel()

	short, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	_, err := s.cancelPodOperation(
		short,
		&types.CancelPodOperationRequest{
			OperationId:   request.GetOperationId(),
			OperationType: types.PodOperationType_POD_CHECKPOINT,
		},
	)

	stop()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = os.Stat(output)
	require.True(t, os.IsNotExist(err))
	close(release)
	require.ErrorIs(t, <-returned, context.Canceled)

	fresh, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()

	response, err := s.cancelPodOperation(
		fresh,
		&types.CancelPodOperationRequest{
			OperationId:   request.GetOperationId(),
			OperationType: types.PodOperationType_POD_CHECKPOINT,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, response)

	data, err := os.ReadFile(output)
	require.NoError(t, err)
	require.Equal(t, "finished", string(data))
}

func TestCancelRestoreChecksUndiscoveredStorage(t *testing.T) {
	for _, test := range []string{"unloaded sandbox", "orphan child", "confirmed absent"} {
		t.Run(test, func(t *testing.T) {
			s := podRestoreTestServer(t)
			store, ok := s.Store().(*containermock.MockStore)
			require.True(t, ok)

			m, err := s.podOperationManager()
			require.NoError(t, err)
			_, err = m.Run(
				context.Background(),
				2,
				"restore",
				"request",
				func(ctx context.Context) ([]byte, error) { return nil, podoperation.SetResource(ctx, "sandbox") },
			)
			require.NoError(t, err)

			switch test {
			case "unloaded sandbox":
				store.EXPECT().Containers().Return(nil, nil)
				store.EXPECT().Container("sandbox").Return(&cstorage.Container{ID: "sandbox"}, nil)
			case "orphan child":
				store.EXPECT().Containers().Return([]cstorage.Container{{ID: "child"}}, nil)
				store.EXPECT().Metadata("child").Return(`{"pod-id":"sandbox"}`, nil)
			case "confirmed absent":
				store.EXPECT().Containers().Return(nil, nil).Times(2)
				store.EXPECT().
					Container("sandbox").
					Return(nil, cstorage.ErrContainerUnknown).
					Times(2)
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			response, err := s.cancelPodOperation(
				ctx,
				&types.CancelPodOperationRequest{
					OperationId:   "restore",
					OperationType: types.PodOperationType_POD_RESTORE,
				},
			)
			if test == "confirmed absent" {
				require.NoError(t, err)
				require.NotNil(t, response)
			} else {
				require.ErrorContains(t, err, "has not been loaded")
				require.Nil(t, response)
			}
		})
	}
}

func TestPodOperationUnsupportedBackendRejectsBeforeAdmission(t *testing.T) {
	s := podRestoreTestServer(t)
	s.config.SetCheckpointRestore(true)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for _, id := range []string{"", "restore-operation"} {
		_, checkpointErr := s.CheckpointPod(ctx, &types.CheckpointPodRequest{OperationId: id})
		_, restoreErr := s.RestorePod(ctx, &types.RestorePodRequest{OperationId: id})

		want := codes.Unimplemented
		if id == "" {
			want = codes.InvalidArgument
		}

		require.Equal(t, want, status.Code(checkpointErr))
		require.Equal(t, want, status.Code(restoreErr))
	}

	_, err := os.Stat(filepath.Join(s.Store().GraphRoot(), "crio", "pod-operations"))
	require.True(
		t,
		os.IsNotExist(err),
		"unsupported backend must reject before durable admission or resource creation",
	)
}

func TestPodOperationCallerDisappearanceSchedulesCleanup(t *testing.T) {
	s := podRestoreTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	request := &types.RestorePodRequest{
		OperationId: "restore-operation",
		Config: &types.PodSandboxConfig{
			Metadata: &types.PodSandboxMetadata{Name: "pod", Namespace: "ns", Uid: "uid"},
		},
	}
	entered := make(chan struct{})
	returned := make(chan error, 1)

	go func() {
		_, err := s.runPodOperation(
			ctx,
			types.PodOperationType_POD_RESTORE,
			request.GetOperationId(),
			request,
			func(owner context.Context) (proto.Message, error) {
				close(entered)
				<-owner.Done()

				return nil, owner.Err()
			},
		)
		returned <- err
	}()

	<-entered
	cancel()
	require.ErrorIs(t, <-returned, context.Canceled)

	directory := filepath.Join(s.Store().GraphRoot(), "crio", "pod-operations")

	require.Eventually(t, func() bool {
		record, err := podoperation.Read(
			directory,
			int32(types.PodOperationType_POD_RESTORE),
			request.GetOperationId(),
		)

		return err == nil && record.Canceled && record.Settled
	}, time.Second, time.Millisecond, "runtime-owned cleanup must not require a second client RPC")
}

func TestDuplicateRestoreDoesNotCleanExistingSandbox(t *testing.T) {
	s := podRestoreTestServer(t)
	_, err := s.ReservePodName("existing-sandbox", "existing-name")
	require.NoError(t, err)

	request := &types.RestorePodRequest{
		OperationId: "duplicate-operation",
		Config: &types.PodSandboxConfig{
			Metadata: &types.PodSandboxMetadata{Name: "pod", Namespace: "ns", Uid: "uid"},
		},
	}
	transaction := &podRestoreTransaction{
		Version:     podRestoreTransactionVersion,
		Key:         podRestoreKey(request.GetConfig().GetMetadata()),
		SandboxName: "existing-name",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = s.runPodOperation(
		ctx,
		types.PodOperationType_POD_RESTORE,
		request.GetOperationId(),
		request,
		func(context.Context) (proto.Message, error) {
			err := s.requirePodRestoreTarget(transaction)

			return nil, err
		},
	)
	require.Equal(t, codes.AlreadyExists, status.Code(err))

	directory := filepath.Join(s.Store().GraphRoot(), "crio", "pod-operations")

	require.Eventually(t, func() bool {
		record, err := podoperation.Read(
			directory,
			int32(types.PodOperationType_POD_RESTORE),
			request.GetOperationId(),
		)

		return err == nil && record.Settled && !record.MayHaveEffects && record.ResourceID == ""
	}, time.Second, time.Millisecond)

	id, err := s.PodIDForName("existing-name")
	require.NoError(t, err)
	require.Equal(t, "existing-sandbox", id)
}

func TestCancelRestoreReleasesOnlySavedReceiptAndName(t *testing.T) {
	s := podRestoreTestServer(t)
	store, ok := s.Store().(*containermock.MockStore)
	require.True(t, ok)

	m, err := s.podOperationManager()
	require.NoError(t, err)

	const key = "owned-receipt"

	_, err = m.RunTarget(
		context.Background(),
		2,
		"restore-owner",
		"request",
		"metadata",
		func(ctx context.Context) ([]byte, error) {
			if err := podoperation.SetTransaction(ctx, key); err != nil {
				return nil, err
			}

			return nil, podoperation.SetResource(ctx, "owned-sandbox")
		},
	)
	require.NoError(t, err)
	_, err = s.ReservePodName("owned-sandbox", "owned-name")
	require.NoError(t, err)
	_, err = s.ReservePodName("other-sandbox", "other-name")
	require.NoError(t, err)

	transaction := &podRestoreTransaction{
		Version:     podRestoreTransactionVersion,
		Key:         key,
		SandboxName: "owned-name",
		OperationID: "restore-owner",
		SandboxID:   "owned-sandbox",
	}
	require.NoError(t, s.writePodRestoreTransaction(transaction))
	store.EXPECT().Containers().Return(nil, nil).Times(2)
	store.EXPECT().Container("owned-sandbox").Return(nil, cstorage.ErrContainerUnknown).Times(2)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err = s.cancelPodOperation(
		ctx,
		&types.CancelPodOperationRequest{
			OperationId:   "restore-owner",
			OperationType: types.PodOperationType_POD_RESTORE,
		},
	)
	require.NoError(t, err)
	_, err = s.readPodRestoreTransaction(key)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = s.PodIDForName("owned-name")
	require.Error(t, err)
	owner, err := s.PodIDForName("other-name")
	require.NoError(t, err)
	require.Equal(t, "other-sandbox", owner)
}

func TestPodOperationCleanupRetriesWithoutRequestOwner(t *testing.T) {
	ctx := t.Context()

	attempts := 0

	require.NoError(t, retryPodOperationCleanup(ctx, time.Millisecond, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("durable fence temporarily unwritable")
		}

		return nil
	}))
	require.Equal(t, 3, attempts)
}

func TestDuplicateStartDoesNotAbandonHealthyRestore(t *testing.T) {
	s := podRestoreTestServer(t)
	m, err := s.podOperationManager()
	require.NoError(t, err)
	_, err = m.Run(
		context.Background(),
		2,
		"healthy-restore",
		"request",
		func(context.Context) ([]byte, error) { return nil, nil },
	)
	require.NoError(t, err)

	const key = "healthy-receipt"

	transaction := &podRestoreTransaction{
		Version:     podRestoreTransactionVersion,
		Key:         key,
		SandboxName: "sandbox-name",
		SandboxID:   "sandbox",
		Fingerprint: "request",
		OperationID: "healthy-restore",
		Response:    &types.RestorePodResponse{PodSandboxId: "sandbox"},
	}
	require.NoError(t, s.writePodRestoreTransaction(transaction))

	builder := sandbox.NewBuilder()
	builder.SetID("sandbox")
	builder.SetName("sandbox-name")
	builder.SetLogDir(t.TempDir())
	builder.SetShmPath("")
	builder.SetNamespace("ns")
	builder.SetKubeName("pod")
	builder.SetMountLabel("")
	builder.SetProcessLabel("")
	builder.SetCgroupParent("")
	builder.SetRuntimeHandler("")
	builder.SetResolvPath("")
	builder.SetHostname("")
	builder.SetPortMappings([]*hostport.PortMapping{})
	builder.SetHostNetwork(true)
	builder.SetUsernsMode("")
	builder.SetPodLinuxOverhead(nil)
	builder.SetPodLinuxResources(nil)
	builder.SetCreatedAt(time.Now())
	require.NoError(
		t,
		builder.SetCRISandbox(
			"sandbox",
			nil,
			nil,
			&types.PodSandboxMetadata{Name: "pod", Namespace: "ns", Uid: "uid"},
		),
	)
	builder.SetPrivileged(false)
	builder.SetContainers(memorystore.New[*oci.Container]())
	sb, err := builder.GetSandbox()
	require.NoError(t, err)
	require.NoError(t, s.AddSandbox(context.Background(), sb))
	container, err := oci.NewContainer(
		"healthy-app",
		"app",
		"",
		"",
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		"",
		&types.ContainerMetadata{Name: "app"},
		"sandbox",
		false,
		false,
		false,
		"",
		t.TempDir(),
		time.Now(),
		"",
	)
	require.NoError(t, err)
	container.SetState(
		&oci.ContainerState{
			State:         rspec.State{Status: oci.ContainerStateRunning},
			PodRestoreKey: key,
		},
	)
	container.SetCreated()
	s.AddContainer(context.Background(), container)
	require.NoError(t, s.CtrIDIndex().Add(container.ID()))
	response, err := s.StartContainer(
		context.Background(),
		&types.StartContainerRequest{ContainerId: container.ID()},
	)
	require.Nil(t, response)
	require.ErrorContains(t, err, "is not in created state")
	record, err := podoperation.Read(
		filepath.Join(s.Store().GraphRoot(), "crio", "pod-operations"),
		2,
		"healthy-restore",
	)
	require.NoError(t, err)
	require.True(t, record.Succeeded)
	require.False(t, record.Canceled)
	require.False(t, record.Uncertain)
	require.Same(t, container, s.GetContainer(context.Background(), container.ID()))
}

func TestStaleRestoreContainerRemovalPreservesReusedName(t *testing.T) {
	s := podRestoreTestServer(t)
	m, err := s.podOperationManager()
	require.NoError(t, err)
	stale, err := oci.NewContainer(
		"old-container",
		"reused-name",
		"",
		"",
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		"",
		&types.ContainerMetadata{Name: "app"},
		"old-sandbox",
		false,
		false,
		false,
		"",
		t.TempDir(),
		time.Now(),
		"",
	)
	require.NoError(t, err)
	_, err = s.ReserveContainerName(stale.ID(), stale.Name())
	require.NoError(t, err)
	_, err = m.RunTarget(
		context.Background(),
		2,
		"old-operation",
		"request",
		"metadata",
		func(ctx context.Context) ([]byte, error) { return nil, podoperation.SetResource(ctx, stale.Sandbox()) },
	)
	require.NoError(t, err)
	require.NoError(
		t,
		m.Cancel(
			context.Background(),
			2,
			"old-operation",
			func(ctx context.Context, _ *podoperation.Record) error {
				s.ReleaseContainerName(ctx, stale.Name())

				return nil
			},
		),
	)

	_, err = s.ReserveContainerName("new-container", stale.Name())
	require.NoError(t, err)
	_, err = m.RunTarget(
		context.Background(),
		2,
		"new-operation",
		"new-request",
		"metadata",
		func(ctx context.Context) ([]byte, error) { return nil, podoperation.SetResource(ctx, "new-sandbox") },
	)
	require.NoError(t, err)
	operationID, err := s.podRestoreOperationForSandbox(stale.Sandbox())
	require.NoError(t, err)
	require.NoError(
		t,
		s.removePodRestoreContainer(
			context.Background(),
			operationID,
			stale.ID(),
			func(ctx context.Context) error {
				s.ReleaseContainerName(ctx, stale.Name())

				return nil
			},
		),
	)
	owner, err := s.ContainerIDForName(stale.Name())
	require.NoError(t, err, "stale removal released the fresh container's name")
	require.Equal(t, "new-container", owner)
	require.Equal(
		t,
		"old-operation",
		operationID,
		"settled resource lookup must retain the old owner's fence",
	)
}
