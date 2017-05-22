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

package service

const (
	// AnnotationLoadBalancerSourceRangesKey is the key of the annotation on a service to set allowed ingress ranges on their LoadBalancers
	//
	// It should be a comma-separated list of CIDRs, e.g. `0.0.0.0/0` to
	// allow full access (the default) or `18.0.0.0/8,56.0.0.0/8` to allow
	// access only from the CIDRs currently allocated to MIT & the USPS.
	//
	// Not all cloud providers support this annotation, though AWS & GCE do.
	AnnotationLoadBalancerSourceRangesKey = "service.beta.kubernetes.io/load-balancer-source-ranges"

	// AnnotationValueExternalTrafficLocal Value of annotation to specify local endpoints behavior.
	AnnotationValueExternalTrafficLocal = "OnlyLocal"
	// AnnotationValueExternalTrafficGlobal Value of annotation to specify global (legacy) behavior.
	AnnotationValueExternalTrafficGlobal = "Global"

	// TODO: The beta annotations have been deprecated, remove them when we release k8s 1.8.

	// BetaAnnotationHealthCheckNodePort Annotation specifying the healthcheck nodePort for the service.
	// If not specified, annotation is created by the service api backend with the allocated nodePort.
	// Will use user-specified nodePort value if specified by the client.
	BetaAnnotationHealthCheckNodePort = "service.beta.kubernetes.io/healthcheck-nodeport"

	// BetaAnnotationExternalTraffic An annotation that denotes if this Service desires to route
	// external traffic to local endpoints only. This preserves Source IP and avoids a second hop.
	BetaAnnotationExternalTraffic = "service.beta.kubernetes.io/external-traffic"
)
