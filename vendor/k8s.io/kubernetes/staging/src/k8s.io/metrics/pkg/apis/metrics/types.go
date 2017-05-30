/*
Copyright 2017 The Kubernetes Authors.

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

package metrics

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient=true
// +resourceName=nodes
// +readonly=true
// +nonNamespaced=true

// resource usage metrics of a node.
type NodeMetrics struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// The following fields define time interval from which metrics were
	// collected from the interval [Timestamp-Window, Timestamp].
	Timestamp metav1.Time
	Window    metav1.Duration

	// The memory usage is the memory working set.
	Usage ResourceList
}

// NodeMetricsList is a list of NodeMetrics.
type NodeMetricsList struct {
	metav1.TypeMeta
	// Standard list metadata.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#types-kinds
	metav1.ListMeta

	// List of node metrics.
	Items []NodeMetrics
}

// +genclient=true
// +resourceName=pods
// +readonly=true

// resource usage metrics of a pod.
type PodMetrics struct {
	metav1.TypeMeta
	metav1.ObjectMeta

	// The following fields define time interval from which metrics were
	// collected from the interval [Timestamp-Window, Timestamp].
	Timestamp metav1.Time
	Window    metav1.Duration

	// Metrics for all containers are collected within the same time window.
	Containers []ContainerMetrics
}

// PodMetricsList is a list of PodMetrics.
type PodMetricsList struct {
	metav1.TypeMeta
	// Standard list metadata.
	// More info: http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#types-kinds
	metav1.ListMeta

	// List of pod metrics.
	Items []PodMetrics
}

// resource usage metrics of a container.
type ContainerMetrics struct {
	// Container name corresponding to the one from pod.spec.containers.
	Name string
	// The memory usage is the memory working set.
	Usage ResourceList
}

// NOTE: ResourceName and ResourceList are copied from
// k8s.io/kubernetes/pkg/api/types.go. We cannot depend on
// k8s.io/kubernetes/pkg/api because that creates cyclic dependency between
// k8s.io/metrics and k8s.io/kubernetes. We cannot depend on
// k8s.io/client-go/pkg/api because the package is going to be deprecated soon.
// There is no need to keep them exact copies. Each repo can define its own
// internal objects.

// ResourceList is a set of (resource name, quantity) pairs.
type ResourceList map[ResourceName]resource.Quantity

// ResourceName is the name identifying various resources in a ResourceList.
type ResourceName string
