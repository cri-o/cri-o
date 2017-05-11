/*
Copyright 2014 The Kubernetes Authors.

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

package config

import (
	"sync"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/glog"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/util/config"
)

// Operation is a type of operation of services or endpoints.
type Operation int

// These are the available operation types.
const (
	ADD Operation = iota
	UPDATE
	REMOVE
	SYNCED
)

// ServiceUpdate describes an operation of services, sent on the channel.
// You can add, update or remove single service by setting Op == ADD|UPDATE|REMOVE.
type ServiceUpdate struct {
	Service *api.Service
	Op      Operation
}

// EndpointsUpdate describes an operation of endpoints, sent on the channel.
// You can add, update or remove single endpoints by setting Op == ADD|UPDATE|REMOVE.
type EndpointsUpdate struct {
	Endpoints *api.Endpoints
	Op        Operation
}

// ServiceConfigHandler is an abstract interface of objects which receive update notifications for the set of services.
type ServiceConfigHandler interface {
	// OnServiceUpdate gets called when a configuration has been changed by one of the sources.
	// This is the union of all the configuration sources.
	OnServiceUpdate(services []api.Service)
}

// EndpointsConfigHandler is an abstract interface of objects which receive update notifications for the set of endpoints.
type EndpointsConfigHandler interface {
	// OnEndpointsUpdate gets called when endpoints configuration is changed for a given
	// service on any of the configuration sources. An example is when a new
	// service comes up, or when containers come up or down for an existing service.
	OnEndpointsUpdate(endpoints []api.Endpoints)
}

// EndpointsConfig tracks a set of endpoints configurations.
// It accepts "set", "add" and "remove" operations of endpoints via channels, and invokes registered handlers on change.
type EndpointsConfig struct {
	mux     *config.Mux
	bcaster *config.Broadcaster
	store   *endpointsStore
}

// NewEndpointsConfig creates a new EndpointsConfig.
// It immediately runs the created EndpointsConfig.
func NewEndpointsConfig() *EndpointsConfig {
	// The updates channel is used to send interrupts to the Endpoints handler.
	// It's buffered because we never want to block for as long as there is a
	// pending interrupt, but don't want to drop them if the handler is doing
	// work.
	updates := make(chan struct{}, 1)
	store := &endpointsStore{updates: updates, endpoints: make(map[string]map[types.NamespacedName]*api.Endpoints)}
	mux := config.NewMux(store)
	bcaster := config.NewBroadcaster()
	go watchForUpdates(bcaster, store, updates)
	return &EndpointsConfig{mux, bcaster, store}
}

// RegisterHandler registers a handler which is called on every endpoints change.
func (c *EndpointsConfig) RegisterHandler(handler EndpointsConfigHandler) {
	c.bcaster.Add(config.ListenerFunc(func(instance interface{}) {
		glog.V(3).Infof("Calling handler.OnEndpointsUpdate()")
		handler.OnEndpointsUpdate(instance.([]api.Endpoints))
	}))
}

// Channel returns a channel to which endpoints updates should be delivered.
func (c *EndpointsConfig) Channel(source string) chan EndpointsUpdate {
	ch := c.mux.Channel(source)
	endpointsCh := make(chan EndpointsUpdate)
	go func() {
		for update := range endpointsCh {
			ch <- update
		}
	}()
	return endpointsCh
}

// Config returns list of all endpoints from underlying store.
func (c *EndpointsConfig) Config() []api.Endpoints {
	return c.store.MergedState().([]api.Endpoints)
}

type endpointsStore struct {
	endpointLock sync.RWMutex
	endpoints    map[string]map[types.NamespacedName]*api.Endpoints
	synced       bool
	updates      chan<- struct{}
}

func (s *endpointsStore) Merge(source string, change interface{}) error {
	s.endpointLock.Lock()
	endpoints := s.endpoints[source]
	if endpoints == nil {
		endpoints = make(map[types.NamespacedName]*api.Endpoints)
	}
	update := change.(EndpointsUpdate)
	switch update.Op {
	case ADD, UPDATE:
		glog.V(5).Infof("Adding new endpoint from source %s : %s", source, spew.Sdump(update.Endpoints))
		name := types.NamespacedName{Namespace: update.Endpoints.Namespace, Name: update.Endpoints.Name}
		endpoints[name] = update.Endpoints
	case REMOVE:
		glog.V(5).Infof("Removing an endpoint %s", spew.Sdump(update.Endpoints))
		name := types.NamespacedName{Namespace: update.Endpoints.Namespace, Name: update.Endpoints.Name}
		delete(endpoints, name)
	case SYNCED:
		s.synced = true
	default:
		glog.V(4).Infof("Received invalid update type: %s", spew.Sdump(update))
	}
	s.endpoints[source] = endpoints
	synced := s.synced
	s.endpointLock.Unlock()
	if s.updates != nil && synced {
		select {
		case s.updates <- struct{}{}:
		default:
			glog.V(4).Infof("Endpoints handler already has a pending interrupt.")
		}
	}
	return nil
}

func (s *endpointsStore) MergedState() interface{} {
	s.endpointLock.RLock()
	defer s.endpointLock.RUnlock()
	endpoints := make([]api.Endpoints, 0)
	for _, sourceEndpoints := range s.endpoints {
		for _, value := range sourceEndpoints {
			endpoints = append(endpoints, *value)
		}
	}
	return endpoints
}

// ServiceConfig tracks a set of service configurations.
// It accepts "set", "add" and "remove" operations of services via channels, and invokes registered handlers on change.
type ServiceConfig struct {
	mux     *config.Mux
	bcaster *config.Broadcaster
	store   *serviceStore
}

// NewServiceConfig creates a new ServiceConfig.
// It immediately runs the created ServiceConfig.
func NewServiceConfig() *ServiceConfig {
	// The updates channel is used to send interrupts to the Services handler.
	// It's buffered because we never want to block for as long as there is a
	// pending interrupt, but don't want to drop them if the handler is doing
	// work.
	updates := make(chan struct{}, 1)
	store := &serviceStore{updates: updates, services: make(map[string]map[types.NamespacedName]*api.Service)}
	mux := config.NewMux(store)
	bcaster := config.NewBroadcaster()
	go watchForUpdates(bcaster, store, updates)
	return &ServiceConfig{mux, bcaster, store}
}

// RegisterHandler registers a handler which is called on every services change.
func (c *ServiceConfig) RegisterHandler(handler ServiceConfigHandler) {
	c.bcaster.Add(config.ListenerFunc(func(instance interface{}) {
		glog.V(3).Infof("Calling handler.OnServiceUpdate()")
		handler.OnServiceUpdate(instance.([]api.Service))
	}))
}

// Channel returns a channel to which services updates should be delivered.
func (c *ServiceConfig) Channel(source string) chan ServiceUpdate {
	ch := c.mux.Channel(source)
	serviceCh := make(chan ServiceUpdate)
	go func() {
		for update := range serviceCh {
			ch <- update
		}
	}()
	return serviceCh
}

// Config returns list of all services from underlying store.
func (c *ServiceConfig) Config() []api.Service {
	return c.store.MergedState().([]api.Service)
}

type serviceStore struct {
	serviceLock sync.RWMutex
	services    map[string]map[types.NamespacedName]*api.Service
	synced      bool
	updates     chan<- struct{}
}

func (s *serviceStore) Merge(source string, change interface{}) error {
	s.serviceLock.Lock()
	services := s.services[source]
	if services == nil {
		services = make(map[types.NamespacedName]*api.Service)
	}
	update := change.(ServiceUpdate)
	switch update.Op {
	case ADD, UPDATE:
		glog.V(5).Infof("Adding new service from source %s : %s", source, spew.Sdump(update.Service))
		name := types.NamespacedName{Namespace: update.Service.Namespace, Name: update.Service.Name}
		services[name] = update.Service
	case REMOVE:
		glog.V(5).Infof("Removing a service %s", spew.Sdump(update.Service))
		name := types.NamespacedName{Namespace: update.Service.Namespace, Name: update.Service.Name}
		delete(services, name)
	case SYNCED:
		s.synced = true
	default:
		glog.V(4).Infof("Received invalid update type: %s", spew.Sdump(update))
	}
	s.services[source] = services
	synced := s.synced
	s.serviceLock.Unlock()
	if s.updates != nil && synced {
		select {
		case s.updates <- struct{}{}:
		default:
			glog.V(4).Infof("Service handler already has a pending interrupt.")
		}
	}
	return nil
}

func (s *serviceStore) MergedState() interface{} {
	s.serviceLock.RLock()
	defer s.serviceLock.RUnlock()
	services := make([]api.Service, 0)
	for _, sourceServices := range s.services {
		for _, value := range sourceServices {
			services = append(services, *value)
		}
	}
	return services
}

// watchForUpdates invokes bcaster.Notify() with the latest version of an object
// when changes occur.
func watchForUpdates(bcaster *config.Broadcaster, accessor config.Accessor, updates <-chan struct{}) {
	for true {
		<-updates
		bcaster.Notify(accessor.MergedState())
	}
}
