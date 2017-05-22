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

package storage

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	etcdtesting "k8s.io/apiserver/pkg/storage/etcd/testing"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/registry/registrytest"
)

func newStorage(t *testing.T) (*REST, *etcdtesting.EtcdTestServer) {
	etcdStorage, server := registrytest.NewEtcdStorage(t, "extensions")
	restOptions := generic.RESTOptions{
		StorageConfig:           etcdStorage,
		Decorator:               generic.UndecoratedStorage,
		DeleteCollectionWorkers: 1,
		ResourcePrefix:          "networkpolicies",
	}
	return NewREST(restOptions), server
}

// createNetworkPolicy is a helper function that returns a NetworkPolicy with the updated resource version.
func createNetworkPolicy(storage *REST, np extensions.NetworkPolicy, t *testing.T) (extensions.NetworkPolicy, error) {
	ctx := genericapirequest.WithNamespace(genericapirequest.NewContext(), np.Namespace)
	obj, err := storage.Create(ctx, &np)
	if err != nil {
		t.Errorf("Failed to create NetworkPolicy, %v", err)
	}
	newNP := obj.(*extensions.NetworkPolicy)
	return *newNP, nil
}

func validNewNetworkPolicy() *extensions.NetworkPolicy {
	port := intstr.FromInt(80)
	return &extensions.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
			Labels:    map[string]string{"a": "b"},
		},
		Spec: extensions.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"a": "b"}},
			Ingress: []extensions.NetworkPolicyIngressRule{
				{
					From: []extensions.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"c": "d"}},
						},
					},
					Ports: []extensions.NetworkPolicyPort{
						{
							Port: &port,
						},
					},
				},
			},
		},
	}
}

var validNetworkPolicy = *validNewNetworkPolicy()

func TestCreate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := registrytest.New(t, storage.Store)
	np := validNewNetworkPolicy()
	np.ObjectMeta = metav1.ObjectMeta{}

	invalidSelector := map[string]string{"NoUppercaseOrSpecialCharsLike=Equals": "b"}
	test.TestCreate(
		// valid
		np,
		// invalid (invalid selector)
		&extensions.NetworkPolicy{
			Spec: extensions.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{MatchLabels: invalidSelector},
				Ingress:     []extensions.NetworkPolicyIngressRule{},
			},
		},
	)
}

func TestUpdate(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := registrytest.New(t, storage.Store)
	test.TestUpdate(
		// valid
		validNewNetworkPolicy(),
		// valid updateFunc
		func(obj runtime.Object) runtime.Object {
			object := obj.(*extensions.NetworkPolicy)
			return object
		},
		// invalid updateFunc
		func(obj runtime.Object) runtime.Object {
			object := obj.(*extensions.NetworkPolicy)
			object.Name = ""
			return object
		},
		func(obj runtime.Object) runtime.Object {
			object := obj.(*extensions.NetworkPolicy)
			object.Spec.PodSelector = metav1.LabelSelector{MatchLabels: map[string]string{}}
			return object
		},
	)
}

func TestDelete(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := registrytest.New(t, storage.Store)
	test.TestDelete(validNewNetworkPolicy())
}

func TestGet(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := registrytest.New(t, storage.Store)
	test.TestGet(validNewNetworkPolicy())
}

func TestList(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := registrytest.New(t, storage.Store)
	test.TestList(validNewNetworkPolicy())
}

func TestWatch(t *testing.T) {
	storage, server := newStorage(t)
	defer server.Terminate(t)
	defer storage.Store.DestroyFunc()
	test := registrytest.New(t, storage.Store)
	test.TestWatch(
		validNewNetworkPolicy(),
		// matching labels
		[]labels.Set{
			{"a": "b"},
		},
		// not matching labels
		[]labels.Set{
			{"a": "c"},
			{"foo": "bar"},
		},
		// matching fields
		[]fields.Set{
			{"metadata.name": "foo"},
		},
		// not matchin fields
		[]fields.Set{
			{"metadata.name": "bar"},
			{"name": "foo"},
		},
	)
}
