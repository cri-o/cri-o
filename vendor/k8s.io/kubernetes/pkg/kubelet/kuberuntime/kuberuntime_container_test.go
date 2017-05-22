/*
Copyright 2016 The Kubernetes Authors.

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

package kuberuntime

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/api/v1"
	runtimeapi "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1"
	containertest "k8s.io/kubernetes/pkg/kubelet/container/testing"
)

// TestRemoveContainer tests removing the container and its corresponding container logs.
func TestRemoveContainer(t *testing.T) {
	fakeRuntime, _, m, err := createTestRuntimeManager()
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "12345678",
			Name:      "bar",
			Namespace: "new",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:            "foo",
					Image:           "busybox",
					ImagePullPolicy: v1.PullIfNotPresent,
				},
			},
		},
	}

	// Create fake sandbox and container
	_, fakeContainers := makeAndSetFakePod(t, m, fakeRuntime, pod)
	assert.Equal(t, len(fakeContainers), 1)

	containerId := fakeContainers[0].Id
	fakeOS := m.osInterface.(*containertest.FakeOS)
	err = m.removeContainer(containerId)
	assert.NoError(t, err)
	// Verify container log is removed
	expectedContainerLogPath := filepath.Join(podLogsRootDirectory, "12345678", "foo_0.log")
	expectedContainerLogSymlink := legacyLogSymlink(containerId, "foo", "bar", "new")
	assert.Equal(t, fakeOS.Removes, []string{expectedContainerLogPath, expectedContainerLogSymlink})
	// Verify container is removed
	assert.Contains(t, fakeRuntime.Called, "RemoveContainer")
	containers, err := fakeRuntime.ListContainers(&runtimeapi.ContainerFilter{Id: containerId})
	assert.NoError(t, err)
	assert.Empty(t, containers)
}
