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

package hostport

import (
	v1 "k8s.io/api/core/v1"
)

// HostPortManager is an interface for adding and removing hostport for a given pod sandbox.
//
//nolint:golint // no reason to change the type name now "type name will be used as hostport.HostPortManager by other packages"
type HostPortManager interface {
	// Add implements port mappings.
	// id should be a unique identifier for a pod, e.g. podSandboxID.
	// name is the human-readable name of the pod.
	// podIP is the IP to add mappings for.
	// hostportMappings are the associated port mappings for the pod.
	Add(id, name, podIP string, hostportMappings []*PortMapping) error
	// Remove cleans up matching port mappings
	// Remove must be able to clean up port mappings without pod IP
	Remove(id string, hostportMappings []*PortMapping) error
}

// PortMapping represents a network port in a container.
type PortMapping struct {
	HostPort      int32
	ContainerPort int32
	Protocol      v1.Protocol
	HostIP        string
}
