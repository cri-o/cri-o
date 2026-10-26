//go:build !linux

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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

func (s *Server) runPodOperation(context.Context, types.PodOperationType, string, proto.Message, func(context.Context) (proto.Message, error)) ([]byte, error) {
	return nil, status.Error(codes.Unimplemented, "Pod operation cancellation is not supported on this platform")
}

func (s *Server) startContainerWithPodOperation(ctx context.Context, request *types.StartContainerRequest) (*types.StartContainerResponse, error) {
	return s.startContainer(ctx, request)
}

func (s *Server) settlePodRestoreBeforeRemoval(context.Context, string) error { return nil }

func (s *Server) podRestoreOperationForSandbox(string) (string, error) { return "", nil }

func (s *Server) removePodRestoreContainer(ctx context.Context, _, _ string, remove func(context.Context) error) error {
	return remove(ctx)
}
