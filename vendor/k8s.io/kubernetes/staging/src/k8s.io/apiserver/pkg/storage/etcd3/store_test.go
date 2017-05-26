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

package etcd3

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"
	"testing"

	apitesting "k8s.io/apimachinery/pkg/api/testing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/apis/example"
	examplev1 "k8s.io/apiserver/pkg/apis/example/v1"
	"k8s.io/apiserver/pkg/storage"
	storagetests "k8s.io/apiserver/pkg/storage/tests"

	"github.com/coreos/etcd/integration"
	"golang.org/x/net/context"
)

var scheme = runtime.NewScheme()
var codecs = serializer.NewCodecFactory(scheme)

func init() {
	metav1.AddToGroupVersion(scheme, metav1.SchemeGroupVersion)
	example.AddToScheme(scheme)
	examplev1.AddToScheme(scheme)
}

// prefixTransformer adds and verifies that all data has the correct prefix on its way in and out.
type prefixTransformer struct {
	prefix []byte
	stale  bool
	err    error
}

func (p prefixTransformer) TransformFromStorage(b []byte) ([]byte, bool, error) {
	if !bytes.HasPrefix(b, p.prefix) {
		return nil, false, fmt.Errorf("value does not have expected prefix: %s", string(b))
	}
	return bytes.TrimPrefix(b, p.prefix), p.stale, p.err
}
func (p prefixTransformer) TransformToStorage(b []byte) ([]byte, error) {
	if len(b) > 0 {
		return append(append([]byte{}, p.prefix...), b...), p.err
	}
	return b, p.err
}

func TestCreate(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)
	etcdClient := cluster.RandClient()

	key := "/testkey"
	out := &example.Pod{}
	obj := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}

	// verify that kv pair is empty before set
	getResp, err := etcdClient.KV.Get(ctx, key)
	if err != nil {
		t.Fatalf("etcdClient.KV.Get failed: %v", err)
	}
	if len(getResp.Kvs) != 0 {
		t.Fatalf("expecting empty result on key: %s", key)
	}

	err = store.Create(ctx, key, obj, out, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	// basic tests of the output
	if obj.ObjectMeta.Name != out.ObjectMeta.Name {
		t.Errorf("pod name want=%s, get=%s", obj.ObjectMeta.Name, out.ObjectMeta.Name)
	}
	if out.ResourceVersion == "" {
		t.Errorf("output should have non-empty resource version")
	}

	// verify that kv pair is not empty after set
	getResp, err = etcdClient.KV.Get(ctx, key)
	if err != nil {
		t.Fatalf("etcdClient.KV.Get failed: %v", err)
	}
	if len(getResp.Kvs) == 0 {
		t.Fatalf("expecting non empty result on key: %s", key)
	}
}

func TestCreateWithTTL(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)

	input := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
	key := "/somekey"

	out := &example.Pod{}
	if err := store.Create(ctx, key, input, out, 1); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	w, err := store.Watch(ctx, key, out.ResourceVersion, storage.Everything)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	testCheckEventType(t, watch.Deleted, w)
}

func TestCreateWithKeyExist(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)
	obj := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
	key, _ := testPropogateStore(ctx, t, store, obj)
	out := &example.Pod{}
	err := store.Create(ctx, key, obj, out, 0)
	if err == nil || !storage.IsNodeExist(err) {
		t.Errorf("expecting key exists error, but get: %s", err)
	}
}

func TestGet(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)
	key, storedObj := testPropogateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}})

	tests := []struct {
		key               string
		ignoreNotFound    bool
		expectNotFoundErr bool
		expectedOut       *example.Pod
	}{{ // test get on existing item
		key:               key,
		ignoreNotFound:    false,
		expectNotFoundErr: false,
		expectedOut:       storedObj,
	}, { // test get on non-existing item with ignoreNotFound=false
		key:               "/non-existing",
		ignoreNotFound:    false,
		expectNotFoundErr: true,
	}, { // test get on non-existing item with ignoreNotFound=true
		key:               "/non-existing",
		ignoreNotFound:    true,
		expectNotFoundErr: false,
		expectedOut:       &example.Pod{},
	}}

	for i, tt := range tests {
		out := &example.Pod{}
		err := store.Get(ctx, tt.key, "", out, tt.ignoreNotFound)
		if tt.expectNotFoundErr {
			if err == nil || !storage.IsNotFound(err) {
				t.Errorf("#%d: expecting not found error, but get: %s", i, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if !reflect.DeepEqual(tt.expectedOut, out) {
			t.Errorf("#%d: pod want=%#v, get=%#v", i, tt.expectedOut, out)
		}
	}
}

func TestUnconditionalDelete(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)
	key, storedObj := testPropogateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}})

	tests := []struct {
		key               string
		expectedObj       *example.Pod
		expectNotFoundErr bool
	}{{ // test unconditional delete on existing key
		key:               key,
		expectedObj:       storedObj,
		expectNotFoundErr: false,
	}, { // test unconditional delete on non-existing key
		key:               "/non-existing",
		expectedObj:       nil,
		expectNotFoundErr: true,
	}}

	for i, tt := range tests {
		out := &example.Pod{} // reset
		err := store.Delete(ctx, tt.key, out, nil)
		if tt.expectNotFoundErr {
			if err == nil || !storage.IsNotFound(err) {
				t.Errorf("#%d: expecting not found error, but get: %s", i, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		if !reflect.DeepEqual(tt.expectedObj, out) {
			t.Errorf("#%d: pod want=%#v, get=%#v", i, tt.expectedObj, out)
		}
	}
}

func TestConditionalDelete(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)
	key, storedObj := testPropogateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", UID: "A"}})

	tests := []struct {
		precondition        *storage.Preconditions
		expectInvalidObjErr bool
	}{{ // test conditional delete with UID match
		precondition:        storage.NewUIDPreconditions("A"),
		expectInvalidObjErr: false,
	}, { // test conditional delete with UID mismatch
		precondition:        storage.NewUIDPreconditions("B"),
		expectInvalidObjErr: true,
	}}

	for i, tt := range tests {
		out := &example.Pod{}
		err := store.Delete(ctx, key, out, tt.precondition)
		if tt.expectInvalidObjErr {
			if err == nil || !storage.IsInvalidObj(err) {
				t.Errorf("#%d: expecting invalid UID error, but get: %s", i, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		if !reflect.DeepEqual(storedObj, out) {
			t.Errorf("#%d: pod want=%#v, get=%#v", i, storedObj, out)
		}
		key, storedObj = testPropogateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", UID: "A"}})
	}
}

func TestGetToList(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)
	key, storedObj := testPropogateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}})

	tests := []struct {
		key         string
		pred        storage.SelectionPredicate
		expectedOut []*example.Pod
	}{{ // test GetToList on existing key
		key:         key,
		pred:        storage.Everything,
		expectedOut: []*example.Pod{storedObj},
	}, { // test GetToList on non-existing key
		key:         "/non-existing",
		pred:        storage.Everything,
		expectedOut: nil,
	}, { // test GetToList with matching pod name
		key: "/non-existing",
		pred: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.ParseSelectorOrDie("metadata.name!=" + storedObj.Name),
			GetAttrs: func(obj runtime.Object) (labels.Set, fields.Set, error) {
				pod := obj.(*example.Pod)
				return nil, fields.Set{"metadata.name": pod.Name}, nil
			},
		},
		expectedOut: nil,
	}}

	for i, tt := range tests {
		out := &example.PodList{}
		err := store.GetToList(ctx, tt.key, "", tt.pred, out)
		if err != nil {
			t.Fatalf("GetToList failed: %v", err)
		}
		if len(out.Items) != len(tt.expectedOut) {
			t.Errorf("#%d: length of list want=%d, get=%d", i, len(tt.expectedOut), len(out.Items))
			continue
		}
		for j, wantPod := range tt.expectedOut {
			getPod := &out.Items[j]
			if !reflect.DeepEqual(wantPod, getPod) {
				t.Errorf("#%d: pod want=%#v, get=%#v", i, wantPod, getPod)
			}
		}
	}
}

func TestGuaranteedUpdate(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)
	key, storeObj := testPropogateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo", UID: "A"}})

	tests := []struct {
		key                 string
		ignoreNotFound      bool
		precondition        *storage.Preconditions
		expectNotFoundErr   bool
		expectInvalidObjErr bool
		expectNoUpdate      bool
		transformStale      bool
	}{{ // GuaranteedUpdate on non-existing key with ignoreNotFound=false
		key:                 "/non-existing",
		ignoreNotFound:      false,
		precondition:        nil,
		expectNotFoundErr:   true,
		expectInvalidObjErr: false,
		expectNoUpdate:      false,
	}, { // GuaranteedUpdate on non-existing key with ignoreNotFound=true
		key:                 "/non-existing",
		ignoreNotFound:      true,
		precondition:        nil,
		expectNotFoundErr:   false,
		expectInvalidObjErr: false,
		expectNoUpdate:      false,
	}, { // GuaranteedUpdate on existing key
		key:                 key,
		ignoreNotFound:      false,
		precondition:        nil,
		expectNotFoundErr:   false,
		expectInvalidObjErr: false,
		expectNoUpdate:      false,
	}, { // GuaranteedUpdate with same data
		key:                 key,
		ignoreNotFound:      false,
		precondition:        nil,
		expectNotFoundErr:   false,
		expectInvalidObjErr: false,
		expectNoUpdate:      true,
	}, { // GuaranteedUpdate with same data but stale
		key:                 key,
		ignoreNotFound:      false,
		precondition:        nil,
		expectNotFoundErr:   false,
		expectInvalidObjErr: false,
		expectNoUpdate:      false,
		transformStale:      true,
	}, { // GuaranteedUpdate with UID match
		key:                 key,
		ignoreNotFound:      false,
		precondition:        storage.NewUIDPreconditions("A"),
		expectNotFoundErr:   false,
		expectInvalidObjErr: false,
		expectNoUpdate:      true,
	}, { // GuaranteedUpdate with UID mismatch
		key:                 key,
		ignoreNotFound:      false,
		precondition:        storage.NewUIDPreconditions("B"),
		expectNotFoundErr:   false,
		expectInvalidObjErr: true,
		expectNoUpdate:      true,
	}}

	for i, tt := range tests {
		out := &example.Pod{}
		name := fmt.Sprintf("foo-%d", i)
		if tt.expectNoUpdate {
			name = storeObj.Name
		}
		originalTransformer := store.transformer.(prefixTransformer)
		if tt.transformStale {
			transformer := originalTransformer
			transformer.stale = true
			store.transformer = transformer
		}
		version := storeObj.ResourceVersion
		err := store.GuaranteedUpdate(ctx, tt.key, out, tt.ignoreNotFound, tt.precondition,
			storage.SimpleUpdate(func(obj runtime.Object) (runtime.Object, error) {
				if tt.expectNotFoundErr && tt.ignoreNotFound {
					if pod := obj.(*example.Pod); pod.Name != "" {
						t.Errorf("#%d: expecting zero value, but get=%#v", i, pod)
					}
				}
				pod := *storeObj
				pod.Name = name
				return &pod, nil
			}))
		store.transformer = originalTransformer

		if tt.expectNotFoundErr {
			if err == nil || !storage.IsNotFound(err) {
				t.Errorf("#%d: expecting not found error, but get: %v", i, err)
			}
			continue
		}
		if tt.expectInvalidObjErr {
			if err == nil || !storage.IsInvalidObj(err) {
				t.Errorf("#%d: expecting invalid UID error, but get: %s", i, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("GuaranteedUpdate failed: %v", err)
		}
		if out.ObjectMeta.Name != name {
			t.Errorf("#%d: pod name want=%s, get=%s", i, name, out.ObjectMeta.Name)
		}
		switch tt.expectNoUpdate {
		case true:
			if version != out.ResourceVersion {
				t.Errorf("#%d: expect no version change, before=%s, after=%s", i, version, out.ResourceVersion)
			}
		case false:
			if version == out.ResourceVersion {
				t.Errorf("#%d: expect version change, but get the same version=%s", i, version)
			}
		}
		storeObj = out
	}
}

func TestGuaranteedUpdateWithTTL(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)

	input := &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
	key := "/somekey"

	out := &example.Pod{}
	err := store.GuaranteedUpdate(ctx, key, out, true, nil,
		func(_ runtime.Object, _ storage.ResponseMeta) (runtime.Object, *uint64, error) {
			ttl := uint64(1)
			return input, &ttl, nil
		})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	w, err := store.Watch(ctx, key, out.ResourceVersion, storage.Everything)
	if err != nil {
		t.Fatalf("Watch failed: %v", err)
	}
	testCheckEventType(t, watch.Deleted, w)
}

func TestGuaranteedUpdateWithConflict(t *testing.T) {
	ctx, store, cluster := testSetup(t)
	defer cluster.Terminate(t)
	key, _ := testPropogateStore(ctx, t, store, &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}})

	errChan := make(chan error, 1)
	var firstToFinish sync.WaitGroup
	var secondToEnter sync.WaitGroup
	firstToFinish.Add(1)
	secondToEnter.Add(1)

	go func() {
		err := store.GuaranteedUpdate(ctx, key, &example.Pod{}, false, nil,
			storage.SimpleUpdate(func(obj runtime.Object) (runtime.Object, error) {
				pod := obj.(*example.Pod)
				pod.Name = "foo-1"
				secondToEnter.Wait()
				return pod, nil
			}))
		firstToFinish.Done()
		errChan <- err
	}()

	updateCount := 0
	err := store.GuaranteedUpdate(ctx, key, &example.Pod{}, false, nil,
		storage.SimpleUpdate(func(obj runtime.Object) (runtime.Object, error) {
			if updateCount == 0 {
				secondToEnter.Done()
				firstToFinish.Wait()
			}
			updateCount++
			pod := obj.(*example.Pod)
			pod.Name = "foo-2"
			return pod, nil
		}))
	if err != nil {
		t.Fatalf("Second GuaranteedUpdate error %#v", err)
	}
	if err := <-errChan; err != nil {
		t.Fatalf("First GuaranteedUpdate error %#v", err)
	}

	if updateCount != 2 {
		t.Errorf("Should have conflict and called update func twice")
	}
}

func TestTransformationFailure(t *testing.T) {
	codec := apitesting.TestCodec(codecs, examplev1.SchemeGroupVersion)
	cluster := integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	defer cluster.Terminate(t)
	store := newStore(cluster.RandClient(), false, codec, "", prefixTransformer{prefix: []byte("test!")})
	ctx := context.Background()

	preset := []struct {
		key       string
		obj       *example.Pod
		storedObj *example.Pod
	}{{
		key: "/one-level/test",
		obj: &example.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "bar"},
			Spec:       storagetests.DeepEqualSafePodSpec(),
		},
	}, {
		key: "/two-level/1/test",
		obj: &example.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "baz"},
			Spec:       storagetests.DeepEqualSafePodSpec(),
		},
	}}
	for i, ps := range preset[:1] {
		preset[i].storedObj = &example.Pod{}
		err := store.Create(ctx, ps.key, ps.obj, preset[:1][i].storedObj, 0)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	// create a second resource with an invalid prefix
	oldTransformer := store.transformer
	store.transformer = prefixTransformer{prefix: []byte("otherprefix!")}
	for i, ps := range preset[1:] {
		preset[1:][i].storedObj = &example.Pod{}
		err := store.Create(ctx, ps.key, ps.obj, preset[1:][i].storedObj, 0)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}
	store.transformer = oldTransformer

	// only the first item is returned, and no error
	var got example.PodList
	if err := store.List(ctx, "/", "", storage.Everything, &got); err != nil {
		t.Errorf("Unexpected error %v", err)
	}
	if e, a := []example.Pod{*preset[0].storedObj}, got.Items; !reflect.DeepEqual(e, a) {
		t.Errorf("Unexpected: %s", diff.ObjectReflectDiff(e, a))
	}

	// Get should fail
	if err := store.Get(ctx, preset[1].key, "", &example.Pod{}, false); !storage.IsInternalError(err) {
		t.Errorf("Unexpected error: %v", err)
	}
	// GuaranteedUpdate without suggestion should return an error
	if err := store.GuaranteedUpdate(ctx, preset[1].key, &example.Pod{}, false, nil, func(input runtime.Object, res storage.ResponseMeta) (output runtime.Object, ttl *uint64, err error) {
		return input, nil, nil
	}); !storage.IsInternalError(err) {
		t.Errorf("Unexpected error: %v", err)
	}
	// GuaranteedUpdate with suggestion should not return an error if we don't change the object
	if err := store.GuaranteedUpdate(ctx, preset[1].key, &example.Pod{}, false, nil, func(input runtime.Object, res storage.ResponseMeta) (output runtime.Object, ttl *uint64, err error) {
		return input, nil, nil
	}, preset[1].obj); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Delete succeeds but reports an error because we cannot access the body
	if err := store.Delete(ctx, preset[1].key, &example.Pod{}, nil); !storage.IsInternalError(err) {
		t.Errorf("Unexpected error: %v", err)
	}

	if err := store.Get(ctx, preset[1].key, "", &example.Pod{}, false); !storage.IsNotFound(err) {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestList(t *testing.T) {
	codec := apitesting.TestCodec(codecs, examplev1.SchemeGroupVersion)
	cluster := integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	defer cluster.Terminate(t)
	store := newStore(cluster.RandClient(), false, codec, "", prefixTransformer{prefix: []byte("test!")})
	ctx := context.Background()

	// Setup storage with the following structure:
	//  /
	//   - one-level/
	//  |            - test
	//  |
	//   - two-level/
	//               - 1/
	//              |   - test
	//              |
	//               - 2/
	//                  - test
	preset := []struct {
		key       string
		obj       *example.Pod
		storedObj *example.Pod
	}{{
		key: "/one-level/test",
		obj: &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
	}, {
		key: "/two-level/1/test",
		obj: &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
	}, {
		key: "/two-level/2/test",
		obj: &example.Pod{ObjectMeta: metav1.ObjectMeta{Name: "bar"}},
	}}

	for i, ps := range preset {
		preset[i].storedObj = &example.Pod{}
		err := store.Create(ctx, ps.key, ps.obj, preset[i].storedObj, 0)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	tests := []struct {
		prefix      string
		pred        storage.SelectionPredicate
		expectedOut []*example.Pod
	}{{ // test List on existing key
		prefix:      "/one-level/",
		pred:        storage.Everything,
		expectedOut: []*example.Pod{preset[0].storedObj},
	}, { // test List on non-existing key
		prefix:      "/non-existing/",
		pred:        storage.Everything,
		expectedOut: nil,
	}, { // test List with pod name matching
		prefix: "/one-level/",
		pred: storage.SelectionPredicate{
			Label: labels.Everything(),
			Field: fields.ParseSelectorOrDie("metadata.name!=" + preset[0].storedObj.Name),
			GetAttrs: func(obj runtime.Object) (labels.Set, fields.Set, error) {
				pod := obj.(*example.Pod)
				return nil, fields.Set{"metadata.name": pod.Name}, nil
			},
		},
		expectedOut: nil,
	}, { // test List with multiple levels of directories and expect flattened result
		prefix:      "/two-level/",
		pred:        storage.Everything,
		expectedOut: []*example.Pod{preset[1].storedObj, preset[2].storedObj},
	}}

	for i, tt := range tests {
		out := &example.PodList{}
		err := store.List(ctx, tt.prefix, "0", tt.pred, out)
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(tt.expectedOut) != len(out.Items) {
			t.Errorf("#%d: length of list want=%d, get=%d", i, len(tt.expectedOut), len(out.Items))
			continue
		}
		for j, wantPod := range tt.expectedOut {
			getPod := &out.Items[j]
			if !reflect.DeepEqual(wantPod, getPod) {
				t.Errorf("#%d: pod want=%#v, get=%#v", i, wantPod, getPod)
			}
		}
	}
}

func testSetup(t *testing.T) (context.Context, *store, *integration.ClusterV3) {
	codec := apitesting.TestCodec(codecs, examplev1.SchemeGroupVersion)
	cluster := integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	store := newStore(cluster.RandClient(), false, codec, "", prefixTransformer{prefix: []byte("test!")})
	ctx := context.Background()
	return ctx, store, cluster
}

// testPropogateStore helps propogates store with objects, automates key generation, and returns
// keys and stored objects.
func testPropogateStore(ctx context.Context, t *testing.T, store *store, obj *example.Pod) (string, *example.Pod) {
	// Setup store with a key and grab the output for returning.
	key := "/testkey"
	setOutput := &example.Pod{}
	err := store.Create(ctx, key, obj, setOutput, 0)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	return key, setOutput
}

func TestPrefix(t *testing.T) {
	codec := apitesting.TestCodec(codecs, examplev1.SchemeGroupVersion)
	cluster := integration.NewClusterV3(t, &integration.ClusterConfig{Size: 1})
	defer cluster.Terminate(t)
	transformer := prefixTransformer{prefix: []byte("test!")}
	testcases := map[string]string{
		"custom/prefix":     "/custom/prefix",
		"/custom//prefix//": "/custom/prefix",
		"/registry":         "/registry",
	}
	for configuredPrefix, effectivePrefix := range testcases {
		store := newStore(cluster.RandClient(), false, codec, configuredPrefix, transformer)
		if store.pathPrefix != effectivePrefix {
			t.Errorf("configured prefix of %s, expected effective prefix of %s, got %s", configuredPrefix, effectivePrefix, store.pathPrefix)
		}
	}
}
