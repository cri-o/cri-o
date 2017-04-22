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

package options

import (
	"fmt"

	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage/storagebackend"
)

const (
	DefaultEtcdPathPrefix = "/registry"
)

type EtcdOptions struct {
	StorageConfig storagebackend.Config

	EtcdServersOverrides []string
}

func NewEtcdOptions(scheme *runtime.Scheme) *EtcdOptions {
	return &EtcdOptions{
		StorageConfig: storagebackend.Config{
			Prefix: DefaultEtcdPathPrefix,
			// Default cache size to 0 - if unset, its size will be set based on target
			// memory usage.
			DeserializationCacheSize: 0,
			Copier: scheme,
		},
	}
}

func (s *EtcdOptions) Validate() []error {
	allErrors := []error{}
	if len(s.StorageConfig.ServerList) == 0 {
		allErrors = append(allErrors, fmt.Errorf("--etcd-servers must be specified"))
	}

	return allErrors
}

// AddEtcdFlags adds flags related to etcd storage for a specific APIServer to the specified FlagSet
func (s *EtcdOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(&s.EtcdServersOverrides, "etcd-servers-overrides", s.EtcdServersOverrides, ""+
		"Per-resource etcd servers overrides, comma separated. The individual override "+
		"format: group/resource#servers, where servers are http://ip:port, semicolon separated.")

	fs.StringVar(&s.StorageConfig.Type, "storage-backend", s.StorageConfig.Type,
		"The storage backend for persistence. Options: 'etcd3' (default), 'etcd2'.")

	fs.IntVar(&s.StorageConfig.DeserializationCacheSize, "deserialization-cache-size", s.StorageConfig.DeserializationCacheSize,
		"Number of deserialized json objects to cache in memory.")

	fs.StringSliceVar(&s.StorageConfig.ServerList, "etcd-servers", s.StorageConfig.ServerList,
		"List of etcd servers to connect with (scheme://ip:port), comma separated.")

	fs.StringVar(&s.StorageConfig.Prefix, "etcd-prefix", s.StorageConfig.Prefix,
		"The prefix to prepend to all resource paths in etcd.")

	fs.StringVar(&s.StorageConfig.KeyFile, "etcd-keyfile", s.StorageConfig.KeyFile,
		"SSL key file used to secure etcd communication.")

	fs.StringVar(&s.StorageConfig.CertFile, "etcd-certfile", s.StorageConfig.CertFile,
		"SSL certification file used to secure etcd communication.")

	fs.StringVar(&s.StorageConfig.CAFile, "etcd-cafile", s.StorageConfig.CAFile,
		"SSL Certificate Authority file used to secure etcd communication.")

	fs.BoolVar(&s.StorageConfig.Quorum, "etcd-quorum-read", s.StorageConfig.Quorum,
		"If true, enable quorum read.")
}
