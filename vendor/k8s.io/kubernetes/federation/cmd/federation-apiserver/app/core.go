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

package app

import (
	"github.com/golang/glog"

	// HACK to ensure that rest mapper from pkg/api is registered for groupName="".
	// This is required because both pkg/api/install and federation/apis/core/install
	// are installing their respective groupMeta at the same groupName.
	// federation/apis/core/install has only a subset of resources and hence if it gets registered first, then installation of v1 API fails in pkg/master.
	// TODO(nikhiljindal): Fix this by ensuring that pkg/api/install and federation/apis/core/install do not conflict with each other.
	_ "k8s.io/kubernetes/pkg/api/install"

	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/kubernetes/federation/apis/core"
	_ "k8s.io/kubernetes/federation/apis/core/install"
	"k8s.io/kubernetes/federation/apis/core/v1"
	"k8s.io/kubernetes/federation/cmd/federation-apiserver/app/options"
	"k8s.io/kubernetes/pkg/api"
	configmapstore "k8s.io/kubernetes/pkg/registry/core/configmap/storage"
	eventstore "k8s.io/kubernetes/pkg/registry/core/event/storage"
	namespacestore "k8s.io/kubernetes/pkg/registry/core/namespace/storage"
	secretstore "k8s.io/kubernetes/pkg/registry/core/secret/storage"
	servicestore "k8s.io/kubernetes/pkg/registry/core/service/storage"
)

func installCoreAPIs(s *options.ServerRunOptions, g *genericapiserver.GenericAPIServer, optsGetter generic.RESTOptionsGetter) {
	serviceStore, serviceStatusStore := servicestore.NewREST(optsGetter)
	namespaceStore, namespaceStatusStore, namespaceFinalizeStore := namespacestore.NewREST(optsGetter)
	secretStore := secretstore.NewREST(optsGetter)
	configMapStore := configmapstore.NewREST(optsGetter)
	eventStore := eventstore.NewREST(optsGetter, uint64(s.EventTTL.Seconds()))

	coreResources := map[string]rest.Storage{
		"secrets":             secretStore,
		"services":            serviceStore,
		"services/status":     serviceStatusStore,
		"namespaces":          namespaceStore,
		"namespaces/status":   namespaceStatusStore,
		"namespaces/finalize": namespaceFinalizeStore,
		"events":              eventStore,
		"configmaps":          configMapStore,
	}
	coreGroupMeta := api.Registry.GroupOrDie(core.GroupName)
	apiGroupInfo := genericapiserver.APIGroupInfo{
		GroupMeta: *coreGroupMeta,
		VersionedResourcesStorageMap: map[string]map[string]rest.Storage{
			v1.SchemeGroupVersion.Version: coreResources,
		},
		OptionsExternalVersion: &api.Registry.GroupOrDie(core.GroupName).GroupVersion,
		Scheme:                 core.Scheme,
		ParameterCodec:         core.ParameterCodec,
		NegotiatedSerializer:   core.Codecs,
	}
	if err := g.InstallLegacyAPIGroup(genericapiserver.DefaultLegacyAPIPrefix, &apiGroupInfo); err != nil {
		glog.Fatalf("Error in registering group version: %+v.\n Error: %v\n", apiGroupInfo, err)
	}
}
