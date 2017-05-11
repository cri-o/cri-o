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

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	pkgruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	restclient "k8s.io/client-go/rest"
	cache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	v1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	v1 "k8s.io/kubernetes/pkg/api/v1"
	kubeclientset "k8s.io/kubernetes/pkg/client/clientset_generated/clientset"

	"reflect"

	"github.com/golang/glog"
	"k8s.io/kubernetes/federation/pkg/federation-controller/util"
	corelisters "k8s.io/kubernetes/pkg/client/listers/core/v1"
)

type clusterCache struct {
	clientset *kubeclientset.Clientset
	cluster   *v1beta1.Cluster
	// A store of services, populated by the serviceController
	serviceStore corelisters.ServiceLister
	// Watches changes to all services
	serviceController cache.Controller
	// A store of endpoint, populated by the serviceController
	endpointStore corelisters.EndpointsLister
	// Watches changes to all endpoints
	endpointController cache.Controller
	// services that need to be synced
	serviceQueue *workqueue.Type
	// endpoints that need to be synced
	endpointQueue *workqueue.Type
}

type clusterClientCache struct {
	rwlock    sync.Mutex // protects serviceMap
	clientMap map[string]*clusterCache
}

func (cc *clusterClientCache) startClusterLW(cluster *v1beta1.Cluster, clusterName string) {
	cachedClusterClient, ok := cc.clientMap[clusterName]
	// only create when no existing cachedClusterClient
	if ok {
		if !reflect.DeepEqual(cachedClusterClient.cluster.Spec, cluster.Spec) {
			//rebuild clientset when cluster spec is changed
			clientset, err := newClusterClientset(cluster)
			if err != nil || clientset == nil {
				glog.Errorf("Failed to create corresponding restclient of kubernetes cluster: %v", err)
			}
			glog.V(4).Infof("Cluster spec changed, rebuild clientset for cluster %s", clusterName)
			cachedClusterClient.clientset = clientset
			go cachedClusterClient.serviceController.Run(wait.NeverStop)
			go cachedClusterClient.endpointController.Run(wait.NeverStop)
			glog.V(2).Infof("Start watching services and endpoints on cluster %s", clusterName)
		} else {
			// do nothing when there is no spec change
			glog.V(4).Infof("Keep clientset for cluster %s", clusterName)
			return
		}
	} else {
		glog.V(4).Infof("No client cache for cluster %s, building new", clusterName)
		clientset, err := newClusterClientset(cluster)
		if err != nil || clientset == nil {
			glog.Errorf("Failed to create corresponding restclient of kubernetes cluster: %v", err)
		}
		cachedClusterClient = &clusterCache{
			cluster:       cluster,
			clientset:     clientset,
			serviceQueue:  workqueue.New(),
			endpointQueue: workqueue.New(),
		}
		var endpointIndexer cache.Indexer
		endpointIndexer, cachedClusterClient.endpointController = cache.NewIndexerInformer(
			&cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (pkgruntime.Object, error) {
					return clientset.Core().Endpoints(metav1.NamespaceAll).List(options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					return clientset.Core().Endpoints(metav1.NamespaceAll).Watch(options)
				},
			},
			&v1.Endpoints{},
			serviceSyncPeriod,
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					cc.enqueueEndpoint(obj, clusterName)
				},
				UpdateFunc: func(old, cur interface{}) {
					cc.enqueueEndpoint(cur, clusterName)
				},
				DeleteFunc: func(obj interface{}) {
					cc.enqueueEndpoint(obj, clusterName)
				},
			},
			cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		)
		cachedClusterClient.endpointStore = corelisters.NewEndpointsLister(endpointIndexer)

		var serviceIndexer cache.Indexer
		serviceIndexer, cachedClusterClient.serviceController = cache.NewIndexerInformer(
			&cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (pkgruntime.Object, error) {
					return clientset.Core().Services(metav1.NamespaceAll).List(options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					return clientset.Core().Services(metav1.NamespaceAll).Watch(options)
				},
			},
			&v1.Service{},
			serviceSyncPeriod,
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					cc.enqueueService(obj, clusterName)
				},
				UpdateFunc: func(old, cur interface{}) {
					oldService, ok := old.(*v1.Service)

					if !ok {
						return
					}
					curService, ok := cur.(*v1.Service)
					if !ok {
						return
					}
					if !reflect.DeepEqual(oldService.Status.LoadBalancer, curService.Status.LoadBalancer) {
						cc.enqueueService(cur, clusterName)
					}
				},
				DeleteFunc: func(obj interface{}) {
					service, _ := obj.(*v1.Service)
					cc.enqueueService(obj, clusterName)
					glog.V(2).Infof("Service %s/%s deletion found and enqueue to service store %s", service.Namespace, service.Name, clusterName)
				},
			},
			cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
		)
		cachedClusterClient.serviceStore = corelisters.NewServiceLister(serviceIndexer)
		cc.clientMap[clusterName] = cachedClusterClient
		go cachedClusterClient.serviceController.Run(wait.NeverStop)
		go cachedClusterClient.endpointController.Run(wait.NeverStop)
		glog.V(2).Infof("Start watching services and endpoints on cluster %s", clusterName)
	}

}

//TODO: copied from cluster controller, to make this as common function in pass 2
// delFromClusterSet delete a cluster from clusterSet and
// delete the corresponding restclient from the map clusterKubeClientMap
func (cc *clusterClientCache) delFromClusterSet(obj interface{}) {
	cluster, ok := obj.(*v1beta1.Cluster)
	cc.rwlock.Lock()
	defer cc.rwlock.Unlock()
	if ok {
		delete(cc.clientMap, cluster.Name)
	} else {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			glog.Infof("Object contained wasn't a cluster or a deleted key: %+v", obj)
			return
		}
		glog.Infof("Found tombstone for %v", obj)
		delete(cc.clientMap, tombstone.Key)
	}
}

// addToClusterSet inserts the new cluster to clusterSet and creates a corresponding
// restclient to map clusterKubeClientMap
func (cc *clusterClientCache) addToClientMap(obj interface{}) {
	cc.rwlock.Lock()
	defer cc.rwlock.Unlock()
	cluster, ok := obj.(*v1beta1.Cluster)
	if !ok {
		return
	}
	pred := getClusterConditionPredicate()
	// check status
	// skip if not ready
	if pred(*cluster) {
		cc.startClusterLW(cluster, cluster.Name)
	}
}

func newClusterClientset(c *v1beta1.Cluster) (*kubeclientset.Clientset, error) {
	clusterConfig, err := util.BuildClusterConfig(c)
	if clusterConfig != nil {
		clientset := kubeclientset.NewForConfigOrDie(restclient.AddUserAgent(clusterConfig, UserAgentName))
		return clientset, nil
	}
	return nil, err
}
