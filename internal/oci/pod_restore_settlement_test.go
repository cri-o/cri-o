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

package oci

import (
	"context"
	"errors"
	"testing"

	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/podoperation"
)

type restoreWithoutSettlement struct {
	RuntimeImpl

	calls int
}

func (r *restoreWithoutSettlement) RestoreContainer(
	context.Context,
	*Container,
	string,
	string,
) error {
	r.calls++

	return nil
}

func TestRestoreOperationRejectsBackendWithoutSettlementBeforeDispatch(t *testing.T) {
	container := &Container{criContainer: &types.Container{Id: "container"}}
	backend := &restoreWithoutSettlement{}
	runtime := &Runtime{runtimeImplMap: map[string]RuntimeImpl{container.ID(): backend}}

	manager, err := podoperation.New(t.TempDir(), "boot")
	if err != nil {
		t.Fatal(err)
	}

	_, err = manager.RunTarget(
		context.Background(),
		2,
		"restore-operation",
		"request",
		"ns/pod/uid/0",
		func(ctx context.Context) ([]byte, error) {
			return nil, runtime.RestoreContainer(ctx, container, "", "")
		},
	)
	if !errors.Is(err, ErrPodRestoreSettlementUnsupported) || backend.calls != 0 {
		t.Fatalf("unsupported restore dispatched: err=%v calls=%d", err, backend.calls)
	}
}

func TestRestoreSettlementDoesNotInstantiateMissingBackend(t *testing.T) {
	container := &Container{criContainer: &types.Container{Id: "container"}}
	runtime := &Runtime{runtimeImplMap: make(map[string]RuntimeImpl)}

	err := runtime.SettlePodRestore(context.Background(), container, "restore-operation")
	if !errors.Is(err, ErrPodRestoreSettlementUnsupported) || len(runtime.runtimeImplMap) != 0 {
		t.Fatalf("recovery instantiated a missing backend: err=%v", err)
	}
}
