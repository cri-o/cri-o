/*
Copyright 2017 The Kubernetes Authors.

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

package configuration

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/apis/admissionregistration/v1alpha1"
)

type mockLister struct {
	invoked           int
	successes         int
	failures          int
	stopCh            chan struct{}
	configurationList v1alpha1.InitializerConfigurationList
	t                 *testing.T
}

func newMockLister(successes, failures int, configurationList v1alpha1.InitializerConfigurationList, t *testing.T) *mockLister {
	return &mockLister{
		failures:          failures,
		successes:         successes,
		configurationList: configurationList,
		stopCh:            make(chan struct{}),
		t:                 t,
	}
}

// The first List will be successful; the next m.failures List will
// fail; the next m.successes List will be successful; the stopCh is closed at
// the 1+m.failures+m.successes call.
func (m *mockLister) List(options metav1.ListOptions) (*v1alpha1.InitializerConfigurationList, error) {
	m.invoked++
	// m.successes could be 0, so call this `if` first.
	if m.invoked == 1+m.failures+m.successes {
		close(m.stopCh)
	}
	if m.invoked == 1 {
		return &m.configurationList, nil
	}
	if m.invoked <= 1+m.failures {
		return nil, fmt.Errorf("some error")
	}
	if m.invoked <= 1+m.failures+m.successes {
		return &m.configurationList, nil
	}
	m.t.Fatalf("unexpected call to List, stopCh has been closed at the %d time call", 1+m.successes+m.failures)
	return nil, nil
}

var _ InitializerConfigurationLister = &mockLister{}

func TestConfiguration(t *testing.T) {
	cases := []struct {
		name     string
		failures int
		// note that the first call to mockLister is always a success.
		successes   int
		expectReady bool
	}{
		{
			name:        "number of failures hasn't reached failureThreshold",
			failures:    defaultFailureThreshold - 1,
			expectReady: true,
		},
		{
			name:        "number of failures just reaches failureThreshold",
			failures:    defaultFailureThreshold,
			expectReady: false,
		},
		{
			name:        "number of failures exceeds failureThreshold",
			failures:    defaultFailureThreshold + 1,
			expectReady: false,
		},
		{
			name:        "number of failures exceeds failureThreshold, but then get another success",
			failures:    defaultFailureThreshold + 1,
			successes:   1,
			expectReady: true,
		},
	}
	for _, c := range cases {
		mock := newMockLister(c.successes, c.failures, v1alpha1.InitializerConfigurationList{}, t)
		manager := NewInitializerConfigurationManager(mock)
		manager.interval = 1 * time.Millisecond
		manager.Run(mock.stopCh)
		<-mock.stopCh
		_, err := manager.Initializers()
		if err != nil && c.expectReady {
			t.Errorf("case %s, expect ready, got: %v", c.name, err)
		}
		if err == nil && !c.expectReady {
			t.Errorf("case %s, expect not ready", c.name)
		}
	}
}

func TestMergeInitializerConfigurations(t *testing.T) {
	configurationsList := v1alpha1.InitializerConfigurationList{
		Items: []v1alpha1.InitializerConfiguration{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "provider_2",
				},
				Initializers: []v1alpha1.Initializer{
					{
						Name: "initializer_a",
					},
					{
						Name: "initializer_b",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "provider_1",
				},
				Initializers: []v1alpha1.Initializer{
					{
						Name: "initializer_c",
					},
					{
						Name: "initializer_d",
					},
				},
			},
		},
	}

	expected := &v1alpha1.InitializerConfiguration{
		Initializers: []v1alpha1.Initializer{
			{
				Name: "initializer_c",
			},
			{
				Name: "initializer_d",
			},
			{
				Name: "initializer_a",
			},
			{
				Name: "initializer_b",
			},
		},
	}

	got := mergeInitializerConfigurations(&configurationsList)
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %#v, got: %#v", expected, got)
	}
}
