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
)

var ErrPodRestoreSettlementUnsupported = errors.New(
	"runtime backend cannot authoritatively settle restored task creation",
)

// PodRestoreSettler is optional because a completed command or a missing task
// does not prove that detached conmon/OCI/CRIU work can no longer act. A backend
// must durably fence this container's work for the operation and join its
// workers before returning nil; sibling containers retain their own owners.
type PodRestoreSettler interface {
	SettlePodRestore(context.Context, *Container, string) error
}

func (r *Runtime) SettlePodRestore(
	ctx context.Context,
	container *Container,
	operationID string,
) error {
	if operationID == "" {
		return errors.New("restore operation identity is required")
	}
	// Recovery must not instantiate a new backend while finding an old worker.
	r.runtimeImplMapMutex.RLock()
	impl := r.runtimeImplMap[container.ID()]
	r.runtimeImplMapMutex.RUnlock()

	backend, ok := impl.(PodRestoreSettler)
	if !ok {
		return ErrPodRestoreSettlementUnsupported
	}

	return backend.SettlePodRestore(ctx, container, operationID)
}
