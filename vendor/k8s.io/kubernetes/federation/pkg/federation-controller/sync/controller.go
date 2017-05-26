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

package sync

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pkgruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	clientv1 "k8s.io/client-go/pkg/api/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	federationapi "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	federationclientset "k8s.io/kubernetes/federation/client/clientset_generated/federation_clientset"
	"k8s.io/kubernetes/federation/pkg/federatedtypes"
	"k8s.io/kubernetes/federation/pkg/federation-controller/util"
	"k8s.io/kubernetes/federation/pkg/federation-controller/util/deletionhelper"
	"k8s.io/kubernetes/federation/pkg/federation-controller/util/eventsink"
	"k8s.io/kubernetes/pkg/api"
	kubeclientset "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"
	"k8s.io/kubernetes/pkg/controller"

	"github.com/golang/glog"
)

const (
	allClustersKey = "ALL_CLUSTERS"
)

// FederationSyncController synchronizes the state of a federated type
// to clusters that are members of the federation.
type FederationSyncController struct {
	// For triggering reconciliation of a single resource. This is
	// used when there is an add/update/delete operation on a resource
	// in either federated API server or in some member of the
	// federation.
	deliverer *util.DelayingDeliverer

	// For triggering reconciliation of all target resources. This is
	// used when a new cluster becomes available.
	clusterDeliverer *util.DelayingDeliverer

	// Contains resources present in members of federation.
	informer util.FederatedInformer
	// For updating members of federation.
	updater util.FederatedUpdater
	// Definitions of resources that should be federated.
	store cache.Store
	// Informer controller for resources that should be federated.
	controller cache.Controller

	// Work queue allowing parallel processing of resources
	workQueue workqueue.Interface

	// Backoff manager
	backoff *flowcontrol.Backoff

	// For events
	eventRecorder record.EventRecorder

	deletionHelper *deletionhelper.DeletionHelper

	reviewDelay           time.Duration
	clusterAvailableDelay time.Duration
	smallDelay            time.Duration
	updateTimeout         time.Duration

	adapter federatedtypes.FederatedTypeAdapter
}

// StartFederationSyncController starts a new sync controller for a type adapter
func StartFederationSyncController(kind string, adapterFactory federatedtypes.AdapterFactory, config *restclient.Config, stopChan <-chan struct{}, minimizeLatency bool) {
	restclient.AddUserAgent(config, fmt.Sprintf("federation-%s-controller", kind))
	client := federationclientset.NewForConfigOrDie(config)
	adapter := adapterFactory(client)
	controller := newFederationSyncController(client, adapter)
	if minimizeLatency {
		controller.minimizeLatency()
	}
	glog.Infof(fmt.Sprintf("Starting federated sync controller for %s resources", kind))
	controller.Run(stopChan)
}

// newFederationSyncController returns a new sync controller for the given client and type adapter
func newFederationSyncController(client federationclientset.Interface, adapter federatedtypes.FederatedTypeAdapter) *FederationSyncController {
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(eventsink.NewFederatedEventSink(client))
	recorder := broadcaster.NewRecorder(api.Scheme, clientv1.EventSource{Component: fmt.Sprintf("federation-%v-controller", adapter.Kind())})

	s := &FederationSyncController{
		reviewDelay:           time.Second * 10,
		clusterAvailableDelay: time.Second * 20,
		smallDelay:            time.Second * 3,
		updateTimeout:         time.Second * 30,
		workQueue:             workqueue.New(),
		backoff:               flowcontrol.NewBackOff(5*time.Second, time.Minute),
		eventRecorder:         recorder,
		adapter:               adapter,
	}

	// Build delivereres for triggering reconciliations.
	s.deliverer = util.NewDelayingDeliverer()
	s.clusterDeliverer = util.NewDelayingDeliverer()

	// Start informer in federated API servers on the resource type that should be federated.
	s.store, s.controller = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (pkgruntime.Object, error) {
				return adapter.FedList(metav1.NamespaceAll, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return adapter.FedWatch(metav1.NamespaceAll, options)
			},
		},
		adapter.ObjectType(),
		controller.NoResyncPeriodFunc(),
		util.NewTriggerOnAllChanges(func(obj pkgruntime.Object) { s.deliverObj(obj, 0, false) }))

	// Federated informer on the resource type in members of federation.
	s.informer = util.NewFederatedInformer(
		client,
		func(cluster *federationapi.Cluster, targetClient kubeclientset.Interface) (cache.Store, cache.Controller) {
			return cache.NewInformer(
				&cache.ListWatch{
					ListFunc: func(options metav1.ListOptions) (pkgruntime.Object, error) {
						return adapter.ClusterList(targetClient, metav1.NamespaceAll, options)
					},
					WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
						return adapter.ClusterWatch(targetClient, metav1.NamespaceAll, options)
					},
				},
				adapter.ObjectType(),
				controller.NoResyncPeriodFunc(),
				// Trigger reconciliation whenever something in federated cluster is changed. In most cases it
				// would be just confirmation that some operation on the target resource type had succeeded.
				util.NewTriggerOnAllChanges(
					func(obj pkgruntime.Object) {
						s.deliverObj(obj, s.reviewDelay, false)
					},
				))
		},

		&util.ClusterLifecycleHandlerFuncs{
			ClusterAvailable: func(cluster *federationapi.Cluster) {
				// When new cluster becomes available process all the target resources again.
				s.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now().Add(s.clusterAvailableDelay))
			},
		},
	)

	// Federated updeater along with Create/Update/Delete operations.
	s.updater = util.NewFederatedUpdater(s.informer, adapter.Kind(), s.updateTimeout, s.eventRecorder,
		func(client kubeclientset.Interface, obj pkgruntime.Object) error {
			_, err := adapter.ClusterCreate(client, obj)
			return err
		},
		func(client kubeclientset.Interface, obj pkgruntime.Object) error {
			_, err := adapter.ClusterUpdate(client, obj)
			return err
		},
		func(client kubeclientset.Interface, obj pkgruntime.Object) error {
			namespacedName := adapter.NamespacedName(obj)
			orphanDependents := false
			err := adapter.ClusterDelete(client, namespacedName, &metav1.DeleteOptions{OrphanDependents: &orphanDependents})
			return err
		})

	s.deletionHelper = deletionhelper.NewDeletionHelper(
		s.updateObject,
		// objNameFunc
		func(obj pkgruntime.Object) string {
			return adapter.NamespacedName(obj).String()
		},
		s.informer,
		s.updater,
	)

	return s
}

// minimizeLatency reduces delays and timeouts to make the controller more responsive (useful for testing).
func (s *FederationSyncController) minimizeLatency() {
	s.clusterAvailableDelay = time.Second
	s.reviewDelay = 50 * time.Millisecond
	s.smallDelay = 20 * time.Millisecond
	s.updateTimeout = 5 * time.Second
}

// Sends the given updated object to apiserver.
func (s *FederationSyncController) updateObject(obj pkgruntime.Object) (pkgruntime.Object, error) {
	return s.adapter.FedUpdate(obj)
}

func (s *FederationSyncController) Run(stopChan <-chan struct{}) {
	go s.controller.Run(stopChan)
	s.informer.Start()
	go func() {
		<-stopChan
		s.informer.Stop()
		s.workQueue.ShutDown()
	}()
	s.deliverer.StartWithHandler(func(item *util.DelayingDelivererItem) {
		s.workQueue.Add(item)
	})
	s.clusterDeliverer.StartWithHandler(func(_ *util.DelayingDelivererItem) {
		s.reconcileOnClusterChange()
	})

	// TODO: Allow multiple workers.
	go wait.Until(s.worker, time.Second, stopChan)

	util.StartBackoffGC(s.backoff, stopChan)
}

type reconciliationStatus int

const (
	statusAllOK reconciliationStatus = iota
	statusNeedsRecheck
	statusError
	statusNotSynced
)

func (s *FederationSyncController) worker() {
	for {
		obj, quit := s.workQueue.Get()
		if quit {
			return
		}

		item := obj.(*util.DelayingDelivererItem)
		namespacedName := item.Value.(*types.NamespacedName)
		status := s.reconcile(*namespacedName)
		s.workQueue.Done(item)

		switch status {
		case statusAllOK:
			break
		case statusError:
			s.deliver(*namespacedName, 0, true)
		case statusNeedsRecheck:
			s.deliver(*namespacedName, s.reviewDelay, false)
		case statusNotSynced:
			s.deliver(*namespacedName, s.reviewDelay, false)
		}
	}
}

func (s *FederationSyncController) deliverObj(obj pkgruntime.Object, delay time.Duration, failed bool) {
	namespacedName := s.adapter.NamespacedName(obj)
	s.deliver(namespacedName, delay, failed)
}

// Adds backoff to delay if this delivery is related to some failure. Resets backoff if there was no failure.
func (s *FederationSyncController) deliver(namespacedName types.NamespacedName, delay time.Duration, failed bool) {
	key := namespacedName.String()
	if failed {
		s.backoff.Next(key, time.Now())
		delay = delay + s.backoff.Get(key)
	} else {
		s.backoff.Reset(key)
	}
	s.deliverer.DeliverAfter(key, &namespacedName, delay)
}

// Check whether all data stores are in sync. False is returned if any of the informer/stores is not yet
// synced with the corresponding api server.
func (s *FederationSyncController) isSynced() bool {
	if !s.informer.ClustersSynced() {
		glog.V(2).Infof("Cluster list not synced")
		return false
	}
	clusters, err := s.informer.GetReadyClusters()
	if err != nil {
		glog.Errorf("Failed to get ready clusters: %v", err)
		return false
	}
	if !s.informer.GetTargetStore().ClustersSynced(clusters) {
		return false
	}
	return true
}

// The function triggers reconciliation of all target federated resources.
func (s *FederationSyncController) reconcileOnClusterChange() {
	if !s.isSynced() {
		s.clusterDeliverer.DeliverAt(allClustersKey, nil, time.Now().Add(s.clusterAvailableDelay))
	}
	for _, obj := range s.store.List() {
		namespacedName := s.adapter.NamespacedName(obj.(pkgruntime.Object))
		s.deliver(namespacedName, s.smallDelay, false)
	}
}

func (s *FederationSyncController) reconcile(namespacedName types.NamespacedName) reconciliationStatus {
	if !s.isSynced() {
		return statusNotSynced
	}

	key := namespacedName.String()
	kind := s.adapter.Kind()
	cachedObj, exist, err := s.store.GetByKey(key)
	if err != nil {
		glog.Errorf("failed to query main %s store for %v: %v", kind, key, err)
		return statusError
	}

	if !exist {
		// Not federated, ignoring.
		return statusAllOK
	}

	// Create a copy before modifying the resource to prevent racing
	// with other readers.
	copiedObj, err := api.Scheme.DeepCopy(cachedObj)
	if err != nil {
		glog.Errorf("error in retrieving %s from store: %v", kind, err)
		return statusError
	}
	if !s.adapter.IsExpectedType(copiedObj) {
		glog.Errorf("object is not the expected type: %v", copiedObj)
		return statusError
	}
	obj := copiedObj.(pkgruntime.Object)
	meta := s.adapter.ObjectMeta(obj)

	if meta.DeletionTimestamp != nil {
		if err := s.delete(obj, namespacedName); err != nil {
			s.eventRecorder.Eventf(obj, api.EventTypeWarning, "DeleteFailed",
				"%s delete failed: %v", strings.ToTitle(kind), err)
			glog.Errorf("failed to delete %s %s: %v", kind, namespacedName, err)
			return statusError
		}
		return statusAllOK
	}

	glog.V(3).Infof("Ensuring delete object from underlying clusters finalizer for %s: %s",
		kind, namespacedName)
	// Add the required finalizers before creating the resource in underlying clusters.
	obj, err = s.deletionHelper.EnsureFinalizers(obj)
	if err != nil {
		glog.Errorf("failed to ensure delete object from underlying clusters finalizer in %s %s: %v",
			kind, namespacedName, err)
		return statusError
	}

	glog.V(3).Infof("Syncing %s %s in underlying clusters", kind, namespacedName)

	clusters, err := s.informer.GetReadyClusters()
	if err != nil {
		glog.Errorf("failed to get cluster list: %v", err)
		return statusNotSynced
	}

	operations := make([]util.FederatedOperation, 0)
	for _, cluster := range clusters {
		clusterObj, found, err := s.informer.GetTargetStore().GetByKey(cluster.Name, key)
		if err != nil {
			glog.Errorf("failed to get %s from %s: %v", key, cluster.Name, err)
			return statusError
		}

		// The data should not be modified.
		desiredObj := s.adapter.Copy(obj)

		if !found {
			operations = append(operations, util.FederatedOperation{
				Type:        util.OperationTypeAdd,
				Obj:         desiredObj,
				ClusterName: cluster.Name,
				Key:         key,
			})
		} else {
			clusterObj := clusterObj.(pkgruntime.Object)

			// Update existing resource, if needed.
			if !s.adapter.Equivalent(desiredObj, clusterObj) {
				operations = append(operations, util.FederatedOperation{
					Type:        util.OperationTypeUpdate,
					Obj:         desiredObj,
					ClusterName: cluster.Name,
					Key:         key,
				})
			}
		}
	}

	if len(operations) == 0 {
		// Everything is in order
		return statusAllOK
	}

	err = s.updater.Update(operations)
	if err != nil {
		glog.Errorf("failed to execute updates for %s: %v", key, err)
		return statusError
	}

	// Evertyhing is in order but let's be double sure
	return statusNeedsRecheck
}

// delete deletes the given resource or returns error if the deletion was not complete.
func (s *FederationSyncController) delete(obj pkgruntime.Object, namespacedName types.NamespacedName) error {
	kind := s.adapter.Kind()
	glog.V(3).Infof("Handling deletion of %s: %v", kind, namespacedName)
	_, err := s.deletionHelper.HandleObjectInUnderlyingClusters(obj)
	if err != nil {
		return err
	}

	err = s.adapter.FedDelete(namespacedName, nil)
	if err != nil {
		// Its all good if the error is not found error. That means it is deleted already and we do not have to do anything.
		// This is expected when we are processing an update as a result of finalizer deletion.
		// The process that deleted the last finalizer is also going to delete the resource and we do not have to do anything.
		if !errors.IsNotFound(err) {
			return fmt.Errorf("failed to delete %s: %v", kind, err)
		}
	}
	return nil
}
