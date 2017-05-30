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

package apiserver

import (
	"net/http"
	"net/url"
	"time"

	"k8s.io/apimachinery/pkg/apimachinery/announced"
	"k8s.io/apimachinery/pkg/apimachinery/registered"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/util/proxy"
	kubeinformers "k8s.io/client-go/informers"
	kubeclientset "k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/pkg/version"

	"k8s.io/kube-aggregator/pkg/apis/apiregistration"
	"k8s.io/kube-aggregator/pkg/apis/apiregistration/install"
	"k8s.io/kube-aggregator/pkg/apis/apiregistration/v1beta1"
	"k8s.io/kube-aggregator/pkg/client/clientset_generated/internalclientset"
	informers "k8s.io/kube-aggregator/pkg/client/informers/internalversion"
	listers "k8s.io/kube-aggregator/pkg/client/listers/apiregistration/internalversion"
	statuscontrollers "k8s.io/kube-aggregator/pkg/controllers/status"
	apiservicestorage "k8s.io/kube-aggregator/pkg/registry/apiservice/etcd"
)

var (
	groupFactoryRegistry = make(announced.APIGroupFactoryRegistry)
	registry             = registered.NewOrDie("")
	Scheme               = runtime.NewScheme()
	Codecs               = serializer.NewCodecFactory(Scheme)
)

func init() {
	install.Install(groupFactoryRegistry, registry, Scheme)

	// we need to add the options (like ListOptions) to empty v1
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Group: "", Version: "v1"})

	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	Scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
}

// legacyAPIServiceName is the fixed name of the only non-groupified API version
const legacyAPIServiceName = "v1."

type ServiceResolver interface {
	ResolveEndpoint(namespace, name string) (*url.URL, error)
}

type aggregatorEndpointRouting struct {
	services  listersv1.ServiceLister
	endpoints listersv1.EndpointsLister
}

type aggregatorClusterRouting struct {
	services listersv1.ServiceLister
}

type Config struct {
	GenericConfig       *genericapiserver.Config
	CoreAPIServerClient kubeclientset.Interface

	// ProxyClientCert/Key are the client cert used to identify this proxy. Backing APIServices use
	// this to confirm the proxy's identity
	ProxyClientCert []byte
	ProxyClientKey  []byte

	// Indicates if the Aggregator should send to the cluster IP (false) or route to the endpoints IP (true)
	EnableAggregatorRouting bool
}

// APIAggregator contains state for a Kubernetes cluster master/api server.
type APIAggregator struct {
	GenericAPIServer *genericapiserver.GenericAPIServer

	delegateHandler http.Handler

	contextMapper genericapirequest.RequestContextMapper

	// proxyClientCert/Key are the client cert used to identify this proxy. Backing APIServices use
	// this to confirm the proxy's identity
	proxyClientCert []byte
	proxyClientKey  []byte

	// proxyHandlers are the proxy handlers that are currently registered, keyed by apiservice.name
	proxyHandlers map[string]*proxyHandler
	// handledGroups are the groups that already have routes
	handledGroups sets.String

	// lister is used to add group handling for /apis/<group> aggregator lookups based on
	// controller state
	lister listers.APIServiceLister

	// provided for easier embedding
	APIRegistrationInformers informers.SharedInformerFactory

	// Information needed to determine routing for the aggregator
	routing ServiceResolver
}

type completedConfig struct {
	*Config
}

// Complete fills in any fields not set that are required to have valid data. It's mutating the receiver.
func (c *Config) Complete() completedConfig {
	// the kube aggregator wires its own discovery mechanism
	// TODO eventually collapse this by extracting all of the discovery out
	c.GenericConfig.EnableDiscovery = false
	c.GenericConfig.Complete()

	version := version.Get()
	c.GenericConfig.Version = &version

	return completedConfig{c}
}

// SkipComplete provides a way to construct a server instance without config completion.
func (c *Config) SkipComplete() completedConfig {
	return completedConfig{c}
}

func (r *aggregatorEndpointRouting) ResolveEndpoint(namespace, name string) (*url.URL, error) {
	return proxy.ResolveEndpoint(r.services, r.endpoints, namespace, name)
}

func (r *aggregatorClusterRouting) ResolveEndpoint(namespace, name string) (*url.URL, error) {
	return proxy.ResolveCluster(r.services, namespace, name)
}

// New returns a new instance of APIAggregator from the given config.
func (c completedConfig) NewWithDelegate(delegationTarget genericapiserver.DelegationTarget) (*APIAggregator, error) {
	genericServer, err := c.Config.GenericConfig.SkipComplete().New("kube-aggregator", delegationTarget) // completion is done in Complete, no need for a second time
	if err != nil {
		return nil, err
	}

	apiregistrationClient, err := internalclientset.NewForConfig(c.Config.GenericConfig.LoopbackClientConfig)
	if err != nil {
		return nil, err
	}
	informerFactory := informers.NewSharedInformerFactory(
		apiregistrationClient,
		5*time.Minute, // this is effectively used as a refresh interval right now.  Might want to do something nicer later on.
	)
	kubeInformers := kubeinformers.NewSharedInformerFactory(c.CoreAPIServerClient, 5*time.Minute)

	var routing ServiceResolver
	if c.EnableAggregatorRouting {
		routing = &aggregatorEndpointRouting{
			services:  kubeInformers.Core().V1().Services().Lister(),
			endpoints: kubeInformers.Core().V1().Endpoints().Lister(),
		}
	} else {
		routing = &aggregatorClusterRouting{
			services: kubeInformers.Core().V1().Services().Lister(),
		}
	}

	s := &APIAggregator{
		GenericAPIServer: genericServer,
		delegateHandler:  delegationTarget.UnprotectedHandler(),
		contextMapper:    c.GenericConfig.RequestContextMapper,
		proxyClientCert:  c.ProxyClientCert,
		proxyClientKey:   c.ProxyClientKey,
		proxyHandlers:    map[string]*proxyHandler{},
		handledGroups:    sets.String{},
		lister:           informerFactory.Apiregistration().InternalVersion().APIServices().Lister(),
		APIRegistrationInformers: informerFactory,
		routing:                  routing,
	}

	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(apiregistration.GroupName, registry, Scheme, metav1.ParameterCodec, Codecs)
	apiGroupInfo.GroupMeta.GroupVersion = v1beta1.SchemeGroupVersion
	v1beta1storage := map[string]rest.Storage{}
	apiServiceREST := apiservicestorage.NewREST(Scheme, c.GenericConfig.RESTOptionsGetter)
	v1beta1storage["apiservices"] = apiServiceREST
	v1beta1storage["apiservices/status"] = apiservicestorage.NewStatusREST(Scheme, apiServiceREST)
	apiGroupInfo.VersionedResourcesStorageMap["v1beta1"] = v1beta1storage

	if err := s.GenericAPIServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return nil, err
	}

	apisHandler := &apisHandler{
		codecs: Codecs,
		lister: s.lister,
		mapper: s.contextMapper,
	}
	s.GenericAPIServer.Handler.NonGoRestfulMux.Handle("/apis", apisHandler)
	s.GenericAPIServer.Handler.NonGoRestfulMux.UnlistedHandle("/apis/", apisHandler)

	apiserviceRegistrationController := NewAPIServiceRegistrationController(informerFactory.Apiregistration().InternalVersion().APIServices(), kubeInformers.Core().V1().Services(), s)
	availableController := statuscontrollers.NewAvailableConditionController(
		informerFactory.Apiregistration().InternalVersion().APIServices(),
		kubeInformers.Core().V1().Services(),
		kubeInformers.Core().V1().Endpoints(),
		apiregistrationClient.Apiregistration(),
	)

	s.GenericAPIServer.AddPostStartHook("start-kube-aggregator-informers", func(context genericapiserver.PostStartHookContext) error {
		informerFactory.Start(context.StopCh)
		kubeInformers.Start(context.StopCh)
		return nil
	})
	s.GenericAPIServer.AddPostStartHook("apiservice-registration-controller", func(context genericapiserver.PostStartHookContext) error {
		go apiserviceRegistrationController.Run(context.StopCh)
		return nil
	})
	s.GenericAPIServer.AddPostStartHook("apiservice-status-available-controller", func(context genericapiserver.PostStartHookContext) error {
		go availableController.Run(context.StopCh)
		return nil
	})

	return s, nil
}

// AddAPIService adds an API service.  It is not thread-safe, so only call it on one thread at a time please.
// It's a slow moving API, so its ok to run the controller on a single thread
func (s *APIAggregator) AddAPIService(apiService *apiregistration.APIService) {
	// if the proxyHandler already exists, it needs to be updated. The aggregation bits do not
	// since they are wired against listers because they require multiple resources to respond
	if proxyHandler, exists := s.proxyHandlers[apiService.Name]; exists {
		proxyHandler.updateAPIService(apiService)
		return
	}

	proxyPath := "/apis/" + apiService.Spec.Group + "/" + apiService.Spec.Version
	// v1. is a special case for the legacy API.  It proxies to a wider set of endpoints.
	if apiService.Name == legacyAPIServiceName {
		proxyPath = "/api"
	}

	// register the proxy handler
	proxyHandler := &proxyHandler{
		contextMapper:   s.contextMapper,
		localDelegate:   s.delegateHandler,
		proxyClientCert: s.proxyClientCert,
		proxyClientKey:  s.proxyClientKey,
		routing:         s.routing,
	}
	proxyHandler.updateAPIService(apiService)
	s.proxyHandlers[apiService.Name] = proxyHandler
	s.GenericAPIServer.Handler.NonGoRestfulMux.Handle(proxyPath, proxyHandler)
	s.GenericAPIServer.Handler.NonGoRestfulMux.UnlistedHandlePrefix(proxyPath+"/", proxyHandler)

	// if we're dealing with the legacy group, we're done here
	if apiService.Name == legacyAPIServiceName {
		return
	}

	// if we've already registered the path with the handler, we don't want to do it again.
	if s.handledGroups.Has(apiService.Spec.Group) {
		return
	}

	// it's time to register the group aggregation endpoint
	groupPath := "/apis/" + apiService.Spec.Group
	groupDiscoveryHandler := &apiGroupHandler{
		codecs:        Codecs,
		groupName:     apiService.Spec.Group,
		lister:        s.lister,
		delegate:      s.delegateHandler,
		contextMapper: s.contextMapper,
	}
	// aggregation is protected
	s.GenericAPIServer.Handler.NonGoRestfulMux.Handle(groupPath, groupDiscoveryHandler)
	s.GenericAPIServer.Handler.NonGoRestfulMux.UnlistedHandle(groupPath+"/", groupDiscoveryHandler)
	s.handledGroups.Insert(apiService.Spec.Group)
}

// RemoveAPIService removes the APIService from being handled.  It is not thread-safe, so only call it on one thread at a time please.
// It's a slow moving API, so its ok to run the controller on a single thread.
func (s *APIAggregator) RemoveAPIService(apiServiceName string) {
	version := apiregistration.APIServiceNameToGroupVersion(apiServiceName)

	proxyPath := "/apis/" + version.Group + "/" + version.Version
	// v1. is a special case for the legacy API.  It proxies to a wider set of endpoints.
	if apiServiceName == legacyAPIServiceName {
		proxyPath = "/api"
	}
	s.GenericAPIServer.Handler.NonGoRestfulMux.Unregister(proxyPath)
	s.GenericAPIServer.Handler.NonGoRestfulMux.Unregister(proxyPath + "/")
	delete(s.proxyHandlers, apiServiceName)

	// TODO unregister group level discovery when there are no more versions for the group
	// We don't need this right away because the handler properly delegates when no versions are present
}
