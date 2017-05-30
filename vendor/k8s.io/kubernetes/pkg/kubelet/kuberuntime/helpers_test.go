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
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/api/v1"
	runtimetesting "k8s.io/kubernetes/pkg/kubelet/apis/cri/testing"
	runtimeapi "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
)

func TestStableKey(t *testing.T) {
	container := &v1.Container{
		Name:  "test_container",
		Image: "foo/image:v1",
	}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test_pod",
			Namespace: "test_pod_namespace",
			UID:       "test_pod_uid",
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{*container},
		},
	}
	oldKey := getStableKey(pod, container)

	// Updating the container image should change the key.
	container.Image = "foo/image:v2"
	newKey := getStableKey(pod, container)
	assert.NotEqual(t, oldKey, newKey)
}

// TestGetSystclsFromAnnotations tests the logic of getting sysctls from annotations.
func TestGetSystclsFromAnnotations(t *testing.T) {
	tests := []struct {
		annotations     map[string]string
		expectedSysctls map[string]string
	}{{
		annotations: map[string]string{
			v1.SysctlsPodAnnotationKey:       "kernel.shmmni=32768,kernel.shmmax=1000000000",
			v1.UnsafeSysctlsPodAnnotationKey: "knet.ipv4.route.min_pmtu=1000",
		},
		expectedSysctls: map[string]string{
			"kernel.shmmni":            "32768",
			"kernel.shmmax":            "1000000000",
			"knet.ipv4.route.min_pmtu": "1000",
		},
	}, {
		annotations: map[string]string{
			v1.SysctlsPodAnnotationKey: "kernel.shmmni=32768,kernel.shmmax=1000000000",
		},
		expectedSysctls: map[string]string{
			"kernel.shmmni": "32768",
			"kernel.shmmax": "1000000000",
		},
	}, {
		annotations: map[string]string{
			v1.UnsafeSysctlsPodAnnotationKey: "knet.ipv4.route.min_pmtu=1000",
		},
		expectedSysctls: map[string]string{
			"knet.ipv4.route.min_pmtu": "1000",
		},
	}}

	for i, test := range tests {
		actualSysctls, err := getSysctlsFromAnnotations(test.annotations)
		assert.NoError(t, err, "TestCase[%d]", i)
		assert.Len(t, actualSysctls, len(test.expectedSysctls), "TestCase[%d]", i)
		assert.Equal(t, test.expectedSysctls, actualSysctls, "TestCase[%d]", i)
	}
}

func TestToKubeContainer(t *testing.T) {
	c := &runtimeapi.Container{
		Id: "test-id",
		Metadata: &runtimeapi.ContainerMetadata{
			Name:    "test-name",
			Attempt: 1,
		},
		Image:    &runtimeapi.ImageSpec{Image: "test-image"},
		ImageRef: "test-image-ref",
		State:    runtimeapi.ContainerState_CONTAINER_RUNNING,
		Annotations: map[string]string{
			containerHashLabel: "1234",
		},
	}
	expect := &kubecontainer.Container{
		ID: kubecontainer.ContainerID{
			Type: runtimetesting.FakeRuntimeName,
			ID:   "test-id",
		},
		Name:    "test-name",
		ImageID: "test-image-ref",
		Image:   "test-image",
		Hash:    uint64(0x1234),
		State:   kubecontainer.ContainerStateRunning,
	}

	_, _, m, err := createTestRuntimeManager()
	assert.NoError(t, err)
	got, err := m.toKubeContainer(c)
	assert.NoError(t, err)
	assert.Equal(t, expect, got)
}
