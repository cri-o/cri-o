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

package cluster

import (
	"strings"
	"time"

	"github.com/golang/glog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	federationv1beta1 "k8s.io/kubernetes/federation/apis/federation/v1beta1"
	clustercache "k8s.io/kubernetes/federation/client/cache"
	federationclientset "k8s.io/kubernetes/federation/client/clientset_generated/federation_clientset"
	"k8s.io/kubernetes/pkg/controller"
)

type ClusterController struct {
	knownClusterSet sets.String

	// federationClient used to operate cluster
	federationClient federationclientset.Interface

	// clusterMonitorPeriod is the period for updating status of cluster
	clusterMonitorPeriod time.Duration
	// clusterClusterStatusMap is a mapping of clusterName and cluster status of last sampling
	clusterClusterStatusMap map[string]federationv1beta1.ClusterStatus

	// clusterKubeClientMap is a mapping of clusterName and restclient
	clusterKubeClientMap map[string]ClusterClient

	// cluster framework and store
	clusterController cache.Controller
	clusterStore      clustercache.StoreToClusterLister
}

// NewclusterController returns a new cluster controller
func NewclusterController(federationClient federationclientset.Interface, clusterMonitorPeriod time.Duration) *ClusterController {
	cc := &ClusterController{
		knownClusterSet:         make(sets.String),
		federationClient:        federationClient,
		clusterMonitorPeriod:    clusterMonitorPeriod,
		clusterClusterStatusMap: make(map[string]federationv1beta1.ClusterStatus),
		clusterKubeClientMap:    make(map[string]ClusterClient),
	}
	cc.clusterStore.Store, cc.clusterController = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return cc.federationClient.Federation().Clusters().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return cc.federationClient.Federation().Clusters().Watch(options)
			},
		},
		&federationv1beta1.Cluster{},
		controller.NoResyncPeriodFunc(),
		cache.ResourceEventHandlerFuncs{
			DeleteFunc: cc.delFromClusterSet,
			AddFunc:    cc.addToClusterSet,
		},
	)
	return cc
}

// delFromClusterSet delete a cluster from clusterSet and
// delete the corresponding restclient from the map clusterKubeClientMap
func (cc *ClusterController) delFromClusterSet(obj interface{}) {
	cluster := obj.(*federationv1beta1.Cluster)
	cc.knownClusterSet.Delete(cluster.Name)
	delete(cc.clusterKubeClientMap, cluster.Name)
}

// addToClusterSet insert the new cluster to clusterSet and create a corresponding
// restclient to map clusterKubeClientMap
func (cc *ClusterController) addToClusterSet(obj interface{}) {
	cluster := obj.(*federationv1beta1.Cluster)
	cc.knownClusterSet.Insert(cluster.Name)
	// create the restclient of cluster
	restClient, err := NewClusterClientSet(cluster)
	if err != nil || restClient == nil {
		glog.Errorf("Failed to create corresponding restclient of kubernetes cluster: %v", err)
		return
	}
	cc.clusterKubeClientMap[cluster.Name] = *restClient
}

// Run begins watching and syncing.
func (cc *ClusterController) Run() {
	defer utilruntime.HandleCrash()
	go cc.clusterController.Run(wait.NeverStop)
	// monitor cluster status periodically, in phase 1 we just get the health state from "/healthz"
	go wait.Until(func() {
		if err := cc.UpdateClusterStatus(); err != nil {
			glog.Errorf("Error monitoring cluster status: %v", err)
		}
	}, cc.clusterMonitorPeriod, wait.NeverStop)
}

func (cc *ClusterController) GetClusterStatus(cluster *federationv1beta1.Cluster) (*federationv1beta1.ClusterStatus, error) {
	// just get the status of cluster, by requesting the restapi "/healthz"
	clusterClient, found := cc.clusterKubeClientMap[cluster.Name]
	if !found {
		glog.Infof("It's a new cluster, a cluster client will be created")
		client, err := NewClusterClientSet(cluster)
		if err != nil || client == nil {
			glog.Errorf("Failed to create cluster client, err: %v", err)
			return nil, err
		}
		clusterClient = *client
		cc.clusterKubeClientMap[cluster.Name] = clusterClient
	}
	clusterStatus := clusterClient.GetClusterHealthStatus()
	return clusterStatus, nil
}

// UpdateClusterStatus checks cluster status and get the metrics from cluster's restapi
func (cc *ClusterController) UpdateClusterStatus() error {
	clusters, err := cc.federationClient.Federation().Clusters().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	for _, cluster := range clusters.Items {
		if !cc.knownClusterSet.Has(cluster.Name) {
			glog.V(1).Infof("ClusterController observed a new cluster: %#v", cluster)
			cc.knownClusterSet.Insert(cluster.Name)
		}
	}

	// If there's a difference between lengths of known clusters and observed clusters
	if len(cc.knownClusterSet) != len(clusters.Items) {
		observedSet := make(sets.String)
		for _, cluster := range clusters.Items {
			observedSet.Insert(cluster.Name)
		}
		deleted := cc.knownClusterSet.Difference(observedSet)
		for clusterName := range deleted {
			glog.V(1).Infof("ClusterController observed a Cluster deletion: %v", clusterName)
			cc.knownClusterSet.Delete(clusterName)
		}
	}
	for _, cluster := range clusters.Items {
		clusterStatusNew, err := cc.GetClusterStatus(&cluster)
		if err != nil {
			glog.Infof("Failed to Get the status of cluster: %v", cluster.Name)
			continue
		}
		clusterStatusOld, found := cc.clusterClusterStatusMap[cluster.Name]
		if !found {
			glog.Infof("There is no status stored for cluster: %v before", cluster.Name)

		} else {
			hasTransition := false
			for i := 0; i < len(clusterStatusNew.Conditions); i++ {
				if !(strings.EqualFold(string(clusterStatusNew.Conditions[i].Type), string(clusterStatusOld.Conditions[i].Type)) &&
					strings.EqualFold(string(clusterStatusNew.Conditions[i].Status), string(clusterStatusOld.Conditions[i].Status))) {
					hasTransition = true
					break
				}
			}
			if !hasTransition {
				for j := 0; j < len(clusterStatusNew.Conditions); j++ {
					clusterStatusNew.Conditions[j].LastTransitionTime = clusterStatusOld.Conditions[j].LastTransitionTime
				}
			}
		}
		clusterClient, found := cc.clusterKubeClientMap[cluster.Name]
		if !found {
			glog.Warningf("Failed to client for cluster %s", cluster.Name)
			continue
		}

		zones, region, err := clusterClient.GetClusterZones()
		if err != nil {
			glog.Warningf("Failed to get zones and region for cluster %s: %v", cluster.Name, err)
			// Don't return err here, as we want the rest of the status update to proceed.
		} else {
			clusterStatusNew.Zones = zones
			clusterStatusNew.Region = region
		}
		cc.clusterClusterStatusMap[cluster.Name] = *clusterStatusNew
		cluster.Status = *clusterStatusNew
		cluster, err := cc.federationClient.Federation().Clusters().UpdateStatus(&cluster)
		if err != nil {
			glog.Warningf("Failed to update the status of cluster: %v ,error is : %v", cluster.Name, err)
			// Don't return err here, as we want to continue processing remaining clusters.
			continue
		}
	}
	return nil
}
