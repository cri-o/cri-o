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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	cstorage "go.podman.io/storage"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/podoperation"
	containermock "github.com/cri-o/cri-o/test/mocks/containerstorage"
)

func TestRemoveMissingRestoreSandboxSettlesJournal(t *testing.T) {
	for _, reopen := range []bool{false, true} {
		name := "same process"
		if reopen {
			name = "reopened journal"
		}

		t.Run(name, func(t *testing.T) {
			s := podRestoreTestServer(t)
			store, ok := s.Store().(*containermock.MockStore)
			require.True(t, ok)
			store.EXPECT().Containers().Return(nil, nil).AnyTimes()
			store.EXPECT().
				Container("removed-sandbox").
				Return(nil, cstorage.ErrContainerUnknown).
				AnyTimes()

			m, err := s.podOperationManager()
			require.NoError(t, err)
			_, err = m.RunTarget(
				context.Background(),
				int32(types.PodOperationType_POD_RESTORE),
				"old-operation",
				"request",
				"target-metadata",
				func(ctx context.Context) ([]byte, error) {
					return nil, podoperation.SetResource(ctx, "removed-sandbox")
				},
			)
			require.NoError(t, err)

			if reopen {
				s = &Server{ContainerServer: s.ContainerServer, config: s.config}
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()

			response, err := s.RemovePodSandbox(
				ctx,
				&types.RemovePodSandboxRequest{PodSandboxId: "removed-sandbox"},
			)

			require.NoError(t, err)
			require.NotNil(t, response)

			m, err = s.podOperationManager()
			require.NoError(t, err)
			_, err = m.RunTarget(
				ctx,
				int32(types.PodOperationType_POD_RESTORE),
				"new-operation",
				"new-request",
				"target-metadata",
				func(context.Context) ([]byte, error) {
					return nil, nil
				},
			)
			require.NoError(
				t,
				err,
				"normal removal must retire the missing sandbox's metadata reservation",
			)
			_, err = m.RunTarget(
				ctx,
				int32(types.PodOperationType_POD_RESTORE),
				"old-operation",
				"request",
				"different-metadata",
				func(context.Context) ([]byte, error) {
					t.Fatal("the removed operation executed again")

					return nil, nil
				},
			)
			require.ErrorIs(
				t,
				err,
				podoperation.ErrExists,
				"retiring the target must retain the old operation's durable fence",
			)
		})
	}
}
