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

type testCase struct {
	id           string
	name         string
	podIP        string
	portMappings []*PortMapping
}

var testCasesV4 = []testCase{
	{
		id:    "0855d5396cdc673af13203c9cc5c95367cad0133306ba4d74d1da6e2876ebe51",
		name:  "pod1_ns1",
		podIP: "10.1.1.2",
		portMappings: []*PortMapping{
			{
				HostPort:      8080,
				ContainerPort: 80,
				Protocol:      v1.ProtocolTCP,
			},
			{
				HostPort:      8081,
				ContainerPort: 81,
				Protocol:      v1.ProtocolUDP,
			},
			{
				HostPort:      8083,
				ContainerPort: 83,
				Protocol:      v1.ProtocolSCTP,
			},
			{
				HostPort:      8084,
				ContainerPort: 84,
				Protocol:      v1.ProtocolTCP,
				HostIP:        "127.0.0.1",
			},
		},
	},
	{
		id:    "2da827da280ff31f6b257138f625d94b90472f614dee4d5f415d99b3e49a2c72",
		name:  "pod3_ns1",
		podIP: "10.1.1.4",
		portMappings: []*PortMapping{
			{
				HostPort:      8443,
				ContainerPort: 443,
				Protocol:      v1.ProtocolTCP,
			},
		},
	},
	{
		// open same HostPort on different HostIPs
		id:    "f51d8a623d1d3d31d6552da3bc080a33ae57ef47daf34c7c5f7d4159d19849b7",
		name:  "pod5_ns5",
		podIP: "10.1.1.5",
		portMappings: []*PortMapping{
			{
				HostPort:      8888,
				ContainerPort: 443,
				Protocol:      v1.ProtocolTCP,
				HostIP:        "127.0.0.2",
			},
			{
				HostPort:      8888,
				ContainerPort: 443,
				Protocol:      v1.ProtocolTCP,
				HostIP:        "127.0.0.1",
			},
		},
	},
	{
		// open same HostPort with different protocols
		id:    "aa6b20dc29d075700fa53f623a00fe4ec8e9042d48f5964e601a1f3257ddc518",
		name:  "pod6_ns1",
		podIP: "10.1.1.6",
		portMappings: []*PortMapping{
			{
				HostPort:      9999,
				ContainerPort: 443,
				Protocol:      v1.ProtocolTCP,
			},
			{
				HostPort:      9999,
				ContainerPort: 443,
				Protocol:      v1.ProtocolUDP,
			},
		},
	},
}

var testCasesV6 = []testCase{
	{
		// Same id and mappings as testCasesV4[0] (but with an IPv6 HostIP)
		id:    "0855d5396cdc673af13203c9cc5c95367cad0133306ba4d74d1da6e2876ebe51",
		name:  "pod1_ns1",
		podIP: "2001:beef::2",
		portMappings: []*PortMapping{
			{
				HostPort:      8080,
				ContainerPort: 80,
				Protocol:      v1.ProtocolTCP,
			},
			{
				HostPort:      8081,
				ContainerPort: 81,
				Protocol:      v1.ProtocolUDP,
			},
			{
				HostPort:      8083,
				ContainerPort: 83,
				Protocol:      v1.ProtocolSCTP,
			},
			{
				HostPort:      8084,
				ContainerPort: 84,
				Protocol:      v1.ProtocolTCP,
				HostIP:        "::1",
			},
		},
	},
	{
		// Same id and mappings as testCasesV4[1]
		id:    "2da827da280ff31f6b257138f625d94b90472f614dee4d5f415d99b3e49a2c72",
		name:  "pod3_ns1",
		podIP: "2001:beef::4",
		portMappings: []*PortMapping{
			{
				HostPort:      8443,
				ContainerPort: 443,
				Protocol:      v1.ProtocolTCP,
			},
		},
	},
}
