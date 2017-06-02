/*
Copyright 2015 The Kubernetes Authors.

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

package thirdpartyresourcedata

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/apis/extensions"
)

type Foo struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" description:"standard object metadata"`

	SomeField  string `json:"someField"`
	OtherField int    `json:"otherField"`
}

func (*Foo) GetObjectKind() schema.ObjectKind {
	return schema.EmptyObjectKind
}

type FooList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" description:"standard list metadata; see http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#metadata"`

	Items []Foo `json:"items"`
}

func TestCodec(t *testing.T) {
	tests := []struct {
		into      runtime.Object
		obj       *Foo
		expectErr bool
		name      string
	}{
		{
			into: &runtime.VersionedObjects{},
			obj: &Foo{
				ObjectMeta: metav1.ObjectMeta{Name: "bar"},
				TypeMeta:   metav1.TypeMeta{APIVersion: "company.com/v1", Kind: "Foo"},
			},
			expectErr: false,
			name:      "versioned objects list",
		},
		{
			obj:       &Foo{ObjectMeta: metav1.ObjectMeta{Name: "bar"}},
			expectErr: true,
			name:      "missing kind",
		},
		{
			obj: &Foo{
				ObjectMeta: metav1.ObjectMeta{Name: "bar"},
				TypeMeta:   metav1.TypeMeta{APIVersion: "company.com/v1", Kind: "Foo"},
			},
			name: "basic",
		},
		{
			into: &extensions.ThirdPartyResourceData{},
			obj: &Foo{
				ObjectMeta: metav1.ObjectMeta{Name: "bar"},
				TypeMeta:   metav1.TypeMeta{Kind: "ThirdPartyResourceData"},
			},
			expectErr: true,
			name:      "broken kind",
		},
		{
			obj: &Foo{
				ObjectMeta: metav1.ObjectMeta{Name: "bar", ResourceVersion: "baz"},
				TypeMeta:   metav1.TypeMeta{APIVersion: "company.com/v1", Kind: "Foo"},
			},
			name: "resource version",
		},
		{
			obj: &Foo{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "bar",
					CreationTimestamp: metav1.Time{Time: time.Unix(100, 0)},
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "company.com/v1",
					Kind:       "Foo",
				},
			},
			name: "creation time",
		},
		{
			obj: &Foo{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "bar",
					ResourceVersion: "baz",
					Labels:          map[string]string{"foo": "bar", "baz": "blah"},
				},
				TypeMeta: metav1.TypeMeta{APIVersion: "company.com/v1", Kind: "Foo"},
			},
			name: "labels",
		},
	}
	api.Registry.AddThirdPartyAPIGroupVersions(schema.GroupVersion{Group: "company.com", Version: "v1"})
	for _, test := range tests {
		d := &thirdPartyResourceDataDecoder{kind: "Foo", delegate: testapi.Extensions.Codec()}
		e := &thirdPartyResourceDataEncoder{gvk: schema.GroupVersionKind{
			Group:   "company.com",
			Version: "v1",
			Kind:    "Foo",
		}, delegate: testapi.Extensions.Codec()}
		data, err := json.Marshal(test.obj)
		if err != nil {
			t.Errorf("[%s] unexpected error: %v", test.name, err)
			continue
		}
		var obj runtime.Object
		if test.into != nil {
			err = runtime.DecodeInto(d, data, test.into)
			obj = test.into
		} else {
			obj, err = runtime.Decode(d, data)
		}
		if err != nil && !test.expectErr {
			t.Errorf("[%s] unexpected error: %v", test.name, err)
			continue
		}
		if test.expectErr {
			if err == nil {
				t.Errorf("[%s] unexpected non-error", test.name)
			}
			continue
		}
		var rsrcObj *extensions.ThirdPartyResourceData
		switch o := obj.(type) {
		case *extensions.ThirdPartyResourceData:
			rsrcObj = o
		case *runtime.VersionedObjects:
			rsrcObj = o.First().(*extensions.ThirdPartyResourceData)
		default:
			t.Errorf("[%s] unexpected object: %v", test.name, obj)
			continue
		}
		if !reflect.DeepEqual(rsrcObj.ObjectMeta, test.obj.ObjectMeta) {
			t.Errorf("[%s]\nexpected\n%v\nsaw\n%v\n", test.name, rsrcObj.ObjectMeta, test.obj.ObjectMeta)
		}
		var output Foo
		if err := json.Unmarshal(rsrcObj.Data, &output); err != nil {
			t.Errorf("[%s] unexpected error: %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(&output, test.obj) {
			t.Errorf("[%s]\nexpected\n%v\nsaw\n%v\n", test.name, test.obj, &output)
		}

		data, err = runtime.Encode(e, rsrcObj)
		if err != nil {
			t.Errorf("[%s] unexpected error: %v", test.name, err)
		}

		var output2 Foo
		if err := json.Unmarshal(data, &output2); err != nil {
			t.Errorf("[%s] unexpected error: %v", test.name, err)
			continue
		}
		if !reflect.DeepEqual(&output2, test.obj) {
			t.Errorf("[%s]\nexpected\n%v\nsaw\n%v\n", test.name, test.obj, &output2)
		}
	}
}

func TestCreater(t *testing.T) {
	creater := NewObjectCreator("creater group", "creater version", api.Scheme)
	tests := []struct {
		name        string
		kind        schema.GroupVersionKind
		expectedObj runtime.Object
		expectErr   bool
	}{
		{
			name:        "valid ThirdPartyResourceData creation",
			kind:        schema.GroupVersionKind{Group: "creater group", Version: "creater version", Kind: "ThirdPartyResourceData"},
			expectedObj: &extensions.ThirdPartyResourceData{},
			expectErr:   false,
		},
		{
			name:        "invalid ThirdPartyResourceData creation",
			kind:        schema.GroupVersionKind{Version: "invalid version", Kind: "ThirdPartyResourceData"},
			expectedObj: nil,
			expectErr:   true,
		},
		{
			name:        "valid ListOptions creation",
			kind:        schema.GroupVersionKind{Version: "v1", Kind: "ListOptions"},
			expectedObj: &metav1.ListOptions{},
			expectErr:   false,
		},
	}
	for _, test := range tests {
		out, err := creater.New(test.kind)
		if err != nil && !test.expectErr {
			t.Errorf("[%s] unexpected error: %v", test.name, err)
		}
		if err == nil && test.expectErr {
			t.Errorf("[%s] unexpected non-error", test.name)
		}
		if !reflect.DeepEqual(test.expectedObj, out) {
			t.Errorf("[%s] unexpected error: expect: %v, got: %v", test.name, test.expectedObj, out)
		}

	}
}

func TestEncodeToStreamForInternalEvent(t *testing.T) {
	e := &thirdPartyResourceDataEncoder{gvk: schema.GroupVersionKind{
		Group:   "company.com",
		Version: "v1",
		Kind:    "Foo",
	}, delegate: testapi.Extensions.Codec()}
	buf := bytes.NewBuffer([]byte{})
	expected := &metav1.WatchEvent{
		Type: "Added",
	}
	err := e.Encode(&metav1.InternalEvent{
		Type: "Added",
	}, buf)

	jBytes, _ := json.Marshal(expected)

	if string(jBytes) == buf.String() {
		t.Errorf("unexpected encoding expected %s got %s", string(jBytes), buf.String())
	}

	if err != nil {
		t.Errorf("unexpected error encoding: %v", err)
	}
}

func TestThirdPartyResourceDataListEncoding(t *testing.T) {
	gv := schema.GroupVersion{Group: "stable.foo.faz", Version: "v1"}
	gvk := gv.WithKind("Bar")
	e := &thirdPartyResourceDataEncoder{delegate: testapi.Extensions.Codec(), gvk: gvk}
	subject := &extensions.ThirdPartyResourceDataList{}

	buf := bytes.NewBuffer([]byte{})
	err := e.Encode(subject, buf)
	if err != nil {
		t.Errorf("encoding unexpected error: %v", err)
	}

	targetOutput := struct {
		Kind       string            `json:"kind,omitempty"`
		Items      []json.RawMessage `json:"items"`
		Metadata   metav1.ListMeta   `json:"metadata,omitempty"`
		APIVersion string            `json:"apiVersion,omitempty"`
	}{}
	err = json.Unmarshal(buf.Bytes(), &targetOutput)

	if err != nil {
		t.Errorf("unmarshal unexpected error: %v", err)
	}

	if expectedKind := gvk.Kind + "List"; expectedKind != targetOutput.Kind {
		t.Errorf("unexpected kind on list got %s expected %s", targetOutput.Kind, expectedKind)
	}

	if targetOutput.Metadata != subject.ListMeta {
		t.Errorf("metadata mismatch %v != %v", targetOutput.Metadata, subject.ListMeta)
	}

	if targetOutput.APIVersion != gv.String() {
		t.Errorf("apiversion mismatch %v != %v", targetOutput.APIVersion, gv.String())
	}
}

func TestDecodeNumbers(t *testing.T) {
	gv := schema.GroupVersion{Group: "stable.foo.faz", Version: "v1"}
	gvk := gv.WithKind("Foo")
	e := &thirdPartyResourceDataEncoder{delegate: testapi.Extensions.Codec(), gvk: gvk}
	d := &thirdPartyResourceDataDecoder{kind: "Foo", delegate: testapi.Extensions.Codec()}

	// Use highest int64 number and 1000000.
	subject := &extensions.ThirdPartyResourceDataList{
		Items: []extensions.ThirdPartyResourceData{
			{
				Data: []byte(`{"num1": 9223372036854775807, "num2": 1000000}`),
			},
		},
	}

	// Encode to get original JSON.
	originalJSON := bytes.NewBuffer([]byte{})
	err := e.Encode(subject, originalJSON)
	if err != nil {
		t.Errorf("encoding unexpected error: %v", err)
	}

	// Decode original JSON.
	var into runtime.Object
	into, _, err = d.Decode(originalJSON.Bytes(), &gvk, into)
	if err != nil {
		t.Errorf("decoding unexpected error: %v", err)
	}

	// Check if int is preserved.
	decodedJSON := into.(*extensions.ThirdPartyResourceDataList).Items[0].Data
	if !strings.Contains(string(decodedJSON), `"num1":9223372036854775807,"num2":1000000`) {
		t.Errorf("Expected %s, got %s", `"num1":9223372036854775807,"num2":1000000`, string(decodedJSON))
	}
}
