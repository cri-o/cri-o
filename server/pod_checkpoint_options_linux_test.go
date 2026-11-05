//go:build linux && test

package server

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/lib"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	containermock "github.com/cri-o/cri-o/test/mocks/containerstorage"
)

func TestPodCheckpointRestoreOptions(t *testing.T) {
	for _, operation := range []string{"checkpoint", "restore"} {
		for _, test := range []struct {
			name    string
			options map[string]string
		}{
			{name: "nil"},
			{name: "empty", options: map[string]string{}},
			{name: "unsupported", options: map[string]string{"future-option": "value"}},
		} {
			t.Run(operation+"/"+test.name, func(t *testing.T) {
				store := containermock.NewMockStore(gomock.NewController(t))
				store.EXPECT().GraphRoot().Return(t.TempDir()).AnyTimes()
				s := &Server{ContainerServer: lib.NewContainerServerForTest(store)}
				s.config.ContainerLevelEnabled = libconfig.ContainerCheckpointRestoreLevelCheckpointRestore
				root := s.Store().GraphRoot()
				ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
				defer cancel()
				path := t.TempDir()

				var err error
				var message string
				if operation == "checkpoint" {
					request := &types.CheckpointPodRequest{
						ContainerIds: []string{"app"}, OutputPath: path, Options: test.options,
					}
					if test.name == "unsupported" {
						request.PodSandboxId = "sandbox"
					}
					response, checkpointErr := s.CheckpointPod(ctx, request)
					require.Nil(t, response)
					err = checkpointErr
					message = "CheckpointPod options are not supported"
				} else {
					request := &types.RestorePodRequest{CheckpointPath: path, Options: test.options}
					if test.name == "unsupported" {
						request.Config = &types.PodSandboxConfig{
							Metadata: &types.PodSandboxMetadata{Name: "pod", Namespace: "ns", Uid: "uid"},
						}
						request.ContainerConfigs = []*types.ContainerConfig{{Metadata: &types.ContainerMetadata{Name: "app"}}}
					}
					// The fixture has no runtime and rejects unexpected storage calls.
					response, restoreErr := s.RestorePod(ctx, request)
					require.Nil(t, response)
					err = restoreErr
					message = "RestorePod options are not supported"
				}
				if test.name == "unsupported" {
					require.Equal(t, codes.InvalidArgument, status.Code(err))
					require.Equal(t, message, status.Convert(err).Message())
				} else if operation == "checkpoint" {
					// Empty options reach sandbox ID validation, without requiring CRIU.
					require.Equal(t, codes.InvalidArgument, status.Code(err))
					require.Equal(t, "pod sandbox ID is required", status.Convert(err).Message())
				} else {
					// Empty options reach restore config validation.
					require.Equal(t, codes.InvalidArgument, status.Code(err))
					require.Equal(t, "sandbox config and metadata are required", status.Convert(err).Message())
				}
				for _, directory := range []string{path, root} {
					entries, err := os.ReadDir(directory)
					require.NoError(t, err)
					require.Empty(t, entries)
				}
			})
		}
	}
}
