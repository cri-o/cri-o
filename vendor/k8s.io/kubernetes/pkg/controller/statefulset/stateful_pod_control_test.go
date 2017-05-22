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

package statefulset

import (
	"errors"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	"k8s.io/kubernetes/pkg/api/v1"
	apps "k8s.io/kubernetes/pkg/apis/apps/v1beta1"
	"k8s.io/kubernetes/pkg/client/clientset_generated/clientset/fake"
	appslisters "k8s.io/kubernetes/pkg/client/listers/apps/v1beta1"
	corelisters "k8s.io/kubernetes/pkg/client/listers/core/v1"
)

func TestStatefulPodControlCreatesPods(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	pvcLister := corelisters.NewPersistentVolumeClaimLister(pvcIndexer)
	control := NewRealStatefulPodControl(fakeClient, nil, nil, pvcLister, recorder)
	fakeClient.AddReactor("get", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), action.GetResource().Resource)
	})
	fakeClient.AddReactor("create", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})
	fakeClient.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})
	if err := control.CreateStatefulPod(set, pod); err != nil {
		t.Errorf("StatefulPodControl failed to create Pod error: %s", err)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 2 {
		t.Errorf("Expected 2 events for successful create found %d", eventCount)
	}
	for i := range events {
		if !strings.Contains(events[i], v1.EventTypeNormal) {
			t.Errorf("Expected normal events found %s", events[i])
		}
	}
}

func TestStatefulPodControlCreatePodExists(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	pvcs := getPersistentVolumeClaims(set, pod)
	pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	for k := range pvcs {
		pvc := pvcs[k]
		pvcIndexer.Add(&pvc)
	}
	pvcLister := corelisters.NewPersistentVolumeClaimLister(pvcIndexer)
	control := NewRealStatefulPodControl(fakeClient, nil, nil, pvcLister, recorder)
	fakeClient.AddReactor("create", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})
	fakeClient.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, pod, apierrors.NewAlreadyExists(action.GetResource().GroupResource(), pod.Name)
	})
	if err := control.CreateStatefulPod(set, pod); !apierrors.IsAlreadyExists(err) {
		t.Errorf("Failed to create Pod error: %s", err)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 0 {
		t.Errorf("Expected 0 events when Pod and PVC exist found %d", eventCount)
		for i := range events {
			t.Log(events[i])
		}
	}
}

func TestStatefulPodControlCreatePodPvcCreateFailure(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	pvcLister := corelisters.NewPersistentVolumeClaimLister(pvcIndexer)
	control := NewRealStatefulPodControl(fakeClient, nil, nil, pvcLister, recorder)
	fakeClient.AddReactor("create", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})
	fakeClient.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})
	if err := control.CreateStatefulPod(set, pod); err == nil {
		t.Error("Failed to produce error on PVC creation failure")
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 2 {
		t.Errorf("Expected 2 events for PVC create failure found %d", eventCount)
	}
	for i := range events {
		if !strings.Contains(events[i], v1.EventTypeWarning) {
			t.Errorf("Expected normal events found %s", events[i])
		}
	}
}

type fakeIndexer struct {
	cache.Indexer
	getError error
}

func (f *fakeIndexer) GetByKey(key string) (interface{}, bool, error) {
	return nil, false, f.getError
}

func TestStatefulPodControlCreatePodPvcGetFailure(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	pvcIndexer := &fakeIndexer{getError: errors.New("API server down")}
	pvcLister := corelisters.NewPersistentVolumeClaimLister(pvcIndexer)
	control := NewRealStatefulPodControl(fakeClient, nil, nil, pvcLister, recorder)
	fakeClient.AddReactor("create", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})
	fakeClient.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})
	if err := control.CreateStatefulPod(set, pod); err == nil {
		t.Error("Failed to produce error on PVC creation failure")
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 2 {
		t.Errorf("Expected 2 events for PVC create failure found %d", eventCount)
	}
	for i := range events {
		if !strings.Contains(events[i], v1.EventTypeWarning) {
			t.Errorf("Expected normal events found %s", events[i])
		}
	}
}

func TestStatefulPodControlCreatePodFailed(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	pvcLister := corelisters.NewPersistentVolumeClaimLister(pvcIndexer)
	control := NewRealStatefulPodControl(fakeClient, nil, nil, pvcLister, recorder)
	fakeClient.AddReactor("create", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		create := action.(core.CreateAction)
		return true, create.GetObject(), nil
	})
	fakeClient.AddReactor("create", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})
	if err := control.CreateStatefulPod(set, pod); err == nil {
		t.Error("Failed to produce error on Pod creation failure")
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 2 {
		t.Errorf("Expected 2 events for failed Pod create found %d", eventCount)
	} else if !strings.Contains(events[0], v1.EventTypeNormal) {
		t.Errorf("Expected normal event found %s", events[0])

	} else if !strings.Contains(events[1], v1.EventTypeWarning) {
		t.Errorf("Expected warning event found %s", events[1])

	}
}

func TestStatefulPodControlNoOpUpdate(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	control := NewRealStatefulPodControl(fakeClient, nil, nil, nil, recorder)
	fakeClient.AddReactor("*", "*", func(action core.Action) (bool, runtime.Object, error) {
		t.Error("no-op update should not make any client invocation")
		return true, nil, apierrors.NewInternalError(errors.New("If we are here we have a problem"))
	})
	if err := control.UpdateStatefulPod(set, pod); err != nil {
		t.Errorf("Error returned on no-op update error: %s", err)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 0 {
		t.Errorf("Expected 0 events for no-op update found %d", eventCount)
	}
}

func TestStatefulPodControlUpdatesIdentity(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := fake.NewSimpleClientset(set, pod)
	control := NewRealStatefulPodControl(fakeClient, nil, nil, nil, recorder)
	var updated *v1.Pod
	fakeClient.PrependReactor("update", "pods", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		updated = update.GetObject().(*v1.Pod)
		return true, update.GetObject(), nil
	})
	pod.Name = "goo-0"
	if err := control.UpdateStatefulPod(set, pod); err != nil {
		t.Errorf("Successful update returned an error: %s", err)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 1 {
		t.Errorf("Expected 1 event for successful Pod update found %d", eventCount)
	} else if !strings.Contains(events[0], v1.EventTypeNormal) {
		t.Errorf("Expected normal event found %s", events[0])
	}
	if !identityMatches(set, updated) {
		t.Error("Name update failed identity does not match")
	}
}

func TestStatefulPodControlUpdateIdentityFailure(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	gooPod := newStatefulSetPod(set, 0)
	gooPod.Name = "goo-0"
	indexer.Add(gooPod)
	podLister := corelisters.NewPodLister(indexer)
	control := NewRealStatefulPodControl(fakeClient, nil, podLister, nil, recorder)
	fakeClient.AddReactor("update", "pods", func(action core.Action) (bool, runtime.Object, error) {
		pod.Name = "goo-0"
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})
	pod.Name = "goo-0"
	if err := control.UpdateStatefulPod(set, pod); err == nil {
		t.Error("Failed update does not generate an error")
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 1 {
		t.Errorf("Expected 1 event for failed Pod update found %d", eventCount)
	} else if !strings.Contains(events[0], v1.EventTypeWarning) {
		t.Errorf("Expected warning event found %s", events[0])
	}
	if identityMatches(set, pod) {
		t.Error("Failed update mutated Pod identity")
	}
}

func TestStatefulPodControlUpdatesPodStorage(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	pvcLister := corelisters.NewPersistentVolumeClaimLister(pvcIndexer)
	control := NewRealStatefulPodControl(fakeClient, nil, nil, pvcLister, recorder)
	pvcs := getPersistentVolumeClaims(set, pod)
	volumes := make([]v1.Volume, len(pod.Spec.Volumes))
	for i := range pod.Spec.Volumes {
		if _, contains := pvcs[pod.Spec.Volumes[i].Name]; !contains {
			volumes = append(volumes, pod.Spec.Volumes[i])
		}
	}
	pod.Spec.Volumes = volumes
	fakeClient.AddReactor("update", "pods", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})
	fakeClient.AddReactor("create", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})
	var updated *v1.Pod
	fakeClient.PrependReactor("update", "pods", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		updated = update.GetObject().(*v1.Pod)
		return true, update.GetObject(), nil
	})
	if err := control.UpdateStatefulPod(set, pod); err != nil {
		t.Errorf("Successful update returned an error: %s", err)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 2 {
		t.Errorf("Expected 2 event for successful Pod storage update found %d", eventCount)
	}
	for i := range events {
		if !strings.Contains(events[i], v1.EventTypeNormal) {
			t.Errorf("Expected normal event found %s", events[i])
		}
	}
	if !storageMatches(set, updated) {
		t.Error("Name update failed identity does not match")
	}
}

func TestStatefulPodControlUpdatePodStorageFailure(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	pvcIndexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	pvcLister := corelisters.NewPersistentVolumeClaimLister(pvcIndexer)
	control := NewRealStatefulPodControl(fakeClient, nil, nil, pvcLister, recorder)
	pvcs := getPersistentVolumeClaims(set, pod)
	volumes := make([]v1.Volume, len(pod.Spec.Volumes))
	for i := range pod.Spec.Volumes {
		if _, contains := pvcs[pod.Spec.Volumes[i].Name]; !contains {
			volumes = append(volumes, pod.Spec.Volumes[i])
		}
	}
	pod.Spec.Volumes = volumes
	fakeClient.AddReactor("update", "pods", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})
	fakeClient.AddReactor("create", "persistentvolumeclaims", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})
	if err := control.UpdateStatefulPod(set, pod); err == nil {
		t.Error("Failed Pod storage update did not return an error")
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 2 {
		t.Errorf("Expected 2 event for failed Pod storage update found %d", eventCount)
	}
	for i := range events {
		if !strings.Contains(events[i], v1.EventTypeWarning) {
			t.Errorf("Expected normal event found %s", events[i])
		}
	}
}

func TestStatefulPodControlUpdatePodConflictSuccess(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	gooPod := newStatefulSetPod(set, 0)
	gooPod.Name = "goo-0"
	indexer.Add(gooPod)
	podLister := corelisters.NewPodLister(indexer)
	control := NewRealStatefulPodControl(fakeClient, nil, podLister, nil, recorder)
	conflict := false
	fakeClient.AddReactor("update", "pods", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		if !conflict {
			conflict = true
			return true, update.GetObject(), apierrors.NewConflict(action.GetResource().GroupResource(), pod.Name, errors.New("conflict"))
		} else {
			return true, update.GetObject(), nil
		}
	})
	pod.Name = "goo-0"
	if err := control.UpdateStatefulPod(set, pod); err != nil {
		t.Errorf("Successful update returned an error: %s", err)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 1 {
		t.Errorf("Expected 1 event for successful Pod update found %d", eventCount)
	} else if !strings.Contains(events[0], v1.EventTypeNormal) {
		t.Errorf("Expected normal event found %s", events[0])
	}
	if !identityMatches(set, pod) {
		t.Error("Name update failed identity does not match")
	}
}

func TestStatefulPodControlUpdatePodConflictFailure(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	updatedPod := newStatefulSetPod(set, 0)
	updatedPod.Spec.Hostname = "wrong"
	indexer.Add(updatedPod)
	podLister := corelisters.NewPodLister(indexer)
	control := NewRealStatefulPodControl(fakeClient, nil, podLister, nil, recorder)
	fakeClient.AddReactor("update", "pods", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), apierrors.NewConflict(action.GetResource().GroupResource(), pod.Name, errors.New("conflict"))

	})
	pod.Name = "goo-0"
	if err := control.UpdateStatefulPod(set, pod); err == nil {
		t.Error("Failed update did not return an error")
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 1 {
		t.Errorf("Expected 1 event for failed Pod update found %d", eventCount)
	} else if !strings.Contains(events[0], v1.EventTypeWarning) {
		t.Errorf("Expected normal event found %s", events[0])
	}
}

func TestStatefulPodControlDeletesStatefulPod(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	control := NewRealStatefulPodControl(fakeClient, nil, nil, nil, recorder)
	fakeClient.AddReactor("delete", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, nil
	})
	if err := control.DeleteStatefulPod(set, pod); err != nil {
		t.Errorf("Error returned on successful delete: %s", err)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 1 {
		t.Errorf("Expected 1 events for successful delete found %d", eventCount)
	} else if !strings.Contains(events[0], v1.EventTypeNormal) {
		t.Errorf("Expected normal event found %s", events[0])
	}
}

func TestStatefulPodControlDeleteFailure(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	pod := newStatefulSetPod(set, 0)
	fakeClient := &fake.Clientset{}
	control := NewRealStatefulPodControl(fakeClient, nil, nil, nil, recorder)
	fakeClient.AddReactor("delete", "pods", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})
	if err := control.DeleteStatefulPod(set, pod); err == nil {
		t.Error("Fialed to return error on failed delete")
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 1 {
		t.Errorf("Expected 1 events for failed delete found %d", eventCount)
	} else if !strings.Contains(events[0], v1.EventTypeWarning) {
		t.Errorf("Expected warning event found %s", events[0])
	}
}

func TestStatefulPodControlUpdatesSetStatus(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	fakeClient := &fake.Clientset{}
	control := NewRealStatefulPodControl(fakeClient, nil, nil, nil, recorder)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), nil
	})
	if err := control.UpdateStatefulSetStatus(set, 2, 1); err != nil {
		t.Errorf("Error returned on successful status update: %s", err)
	}
	if set.Status.Replicas != 2 {
		t.Errorf("UpdateStatefulSetStatus mutated the sets replicas %d", set.Status.Replicas)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 0 {
		t.Errorf("Expected 0 events for successful status update %d", eventCount)
	}
}

func TestStatefulPodControlUpdatesObservedGeneration(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	fakeClient := &fake.Clientset{}
	control := NewRealStatefulPodControl(fakeClient, nil, nil, nil, recorder)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		sts := update.GetObject().(*apps.StatefulSet)
		if sts.Status.ObservedGeneration == nil || *sts.Status.ObservedGeneration != int64(3) {
			t.Errorf("expected observedGeneration to be synced with generation for statefulset %q", sts.Name)
		}
		return true, sts, nil
	})
	if err := control.UpdateStatefulSetStatus(set, 2, 3); err != nil {
		t.Errorf("Error returned on successful status update: %s", err)
	}
}

func TestStatefulPodControlUpdateReplicasFailure(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	fakeClient := &fake.Clientset{}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	indexer.Add(set)
	setLister := appslisters.NewStatefulSetLister(indexer)
	control := NewRealStatefulPodControl(fakeClient, setLister, nil, nil, recorder)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInternalError(errors.New("API server down"))
	})
	if err := control.UpdateStatefulSetStatus(set, 2, 1); err == nil {
		t.Error("Failed update did not return error")
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 0 {
		t.Errorf("Expected 0 events for successful status update %d", eventCount)
	}
}

func TestStatefulPodControlUpdateReplicasConflict(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	conflict := false
	fakeClient := &fake.Clientset{}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	indexer.Add(set)
	setLister := appslisters.NewStatefulSetLister(indexer)
	control := NewRealStatefulPodControl(fakeClient, setLister, nil, nil, recorder)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		if !conflict {
			conflict = true
			return true, update.GetObject(), apierrors.NewConflict(action.GetResource().GroupResource(), set.Name, errors.New("Object already exists"))
		} else {
			return true, update.GetObject(), nil
		}
	})
	if err := control.UpdateStatefulSetStatus(set, 2, 1); err != nil {
		t.Errorf("UpdateStatefulSetStatus returned an error: %s", err)
	}
	if set.Status.Replicas != 2 {
		t.Errorf("UpdateStatefulSetStatus mutated the sets replicas %d", set.Status.Replicas)
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 0 {
		t.Errorf("Expected 0 events for successful status update %d", eventCount)
	}
}

func TestStatefulPodControlUpdateReplicasConflictFailure(t *testing.T) {
	recorder := record.NewFakeRecorder(10)
	set := newStatefulSet(3)
	fakeClient := &fake.Clientset{}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
	indexer.Add(set)
	setLister := appslisters.NewStatefulSetLister(indexer)
	control := NewRealStatefulPodControl(fakeClient, setLister, nil, nil, recorder)
	fakeClient.AddReactor("update", "statefulsets", func(action core.Action) (bool, runtime.Object, error) {
		update := action.(core.UpdateAction)
		return true, update.GetObject(), apierrors.NewConflict(action.GetResource().GroupResource(), set.Name, errors.New("Object already exists"))
	})
	if err := control.UpdateStatefulSetStatus(set, 2, 1); err == nil {
		t.Error("UpdateStatefulSetStatus failed to return an error on get failure")
	}
	events := collectEvents(recorder.Events)
	if eventCount := len(events); eventCount != 0 {
		t.Errorf("Expected 0 events for successful status update %d", eventCount)
	}
}

func collectEvents(source <-chan string) []string {
	done := false
	events := make([]string, 0)
	for !done {
		select {
		case event := <-source:
			events = append(events, event)
		default:
			done = true
		}
	}
	return events
}
