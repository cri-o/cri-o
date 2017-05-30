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

package validation

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/apps"
)

func TestValidateStatefulSet(t *testing.T) {
	validLabels := map[string]string{"a": "b"}
	validPodTemplate := api.PodTemplate{
		Template: api.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: api.PodSpec{
				RestartPolicy: api.RestartPolicyAlways,
				DNSPolicy:     api.DNSClusterFirst,
				Containers:    []api.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
			},
		},
	}
	invalidLabels := map[string]string{"NoUppercaseOrSpecialCharsLike=Equals": "b"}
	invalidPodTemplate := api.PodTemplate{
		Template: api.PodTemplateSpec{
			Spec: api.PodSpec{
				RestartPolicy: api.RestartPolicyAlways,
				DNSPolicy:     api.DNSClusterFirst,
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: invalidLabels,
			},
		},
	}
	successCases := []apps.StatefulSet{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123", Namespace: metav1.NamespaceDefault},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
			},
		},
	}
	for _, successCase := range successCases {
		if errs := ValidateStatefulSet(&successCase); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}

	errorCases := map[string]apps.StatefulSet{
		"zero-length ID": {
			ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
			},
		},
		"missing-namespace": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc-123"},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
			},
		},
		"empty selector": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Template:            validPodTemplate.Template,
			},
		},
		"selector_doesnt_match": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
				Template:            validPodTemplate.Template,
			},
		},
		"invalid manifest": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
			},
		},
		"negative_replicas": {
			ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Replicas:            -1,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
			},
		},
		"invalid_label": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
				Labels: map[string]string{
					"NoUppercaseOrSpecialCharsLike=Equals": "bar",
				},
			},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
			},
		},
		"invalid_label 2": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
				Labels: map[string]string{
					"NoUppercaseOrSpecialCharsLike=Equals": "bar",
				},
			},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Template:            invalidPodTemplate.Template,
			},
		},
		"invalid_annotation": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
				Annotations: map[string]string{
					"NoUppercaseOrSpecialCharsLike=Equals": "bar",
				},
			},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template:            validPodTemplate.Template,
			},
		},
		"invalid restart policy 1": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template: api.PodTemplateSpec{
					Spec: api.PodSpec{
						RestartPolicy: api.RestartPolicyOnFailure,
						DNSPolicy:     api.DNSClusterFirst,
						Containers:    []api.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent"}},
					},
					ObjectMeta: metav1.ObjectMeta{
						Labels: validLabels,
					},
				},
			},
		},
		"invalid restart policy 2": {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "abc-123",
				Namespace: metav1.NamespaceDefault,
			},
			Spec: apps.StatefulSetSpec{
				PodManagementPolicy: apps.OrderedReadyPodManagement,
				Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
				Template: api.PodTemplateSpec{
					Spec: api.PodSpec{
						RestartPolicy: api.RestartPolicyNever,
						DNSPolicy:     api.DNSClusterFirst,
						Containers:    []api.Container{{Name: "ctr", Image: "image", ImagePullPolicy: "IfNotPresent"}},
					},
					ObjectMeta: metav1.ObjectMeta{
						Labels: validLabels,
					},
				},
			},
		},
	}
	for k, v := range errorCases {
		errs := ValidateStatefulSet(&v)
		if len(errs) == 0 {
			t.Errorf("expected failure for %s", k)
		}
		for i := range errs {
			field := errs[i].Field
			if !strings.HasPrefix(field, "spec.template.") &&
				field != "metadata.name" &&
				field != "metadata.namespace" &&
				field != "spec.selector" &&
				field != "spec.template" &&
				field != "GCEPersistentDisk.ReadOnly" &&
				field != "spec.replicas" &&
				field != "spec.template.labels" &&
				field != "metadata.annotations" &&
				field != "metadata.labels" &&
				field != "status.replicas" {
				t.Errorf("%s: missing prefix for: %v", k, errs[i])
			}
		}
	}
}

func TestValidateStatefulSetUpdate(t *testing.T) {
	validLabels := map[string]string{"a": "b"}
	validPodTemplate := api.PodTemplate{
		Template: api.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: api.PodSpec{
				RestartPolicy: api.RestartPolicyAlways,
				DNSPolicy:     api.DNSClusterFirst,
				Containers:    []api.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
			},
		},
	}
	readWriteVolumePodTemplate := api.PodTemplate{
		Template: api.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: validLabels,
			},
			Spec: api.PodSpec{
				RestartPolicy: api.RestartPolicyAlways,
				DNSPolicy:     api.DNSClusterFirst,
				Containers:    []api.Container{{Name: "abc", Image: "image", ImagePullPolicy: "IfNotPresent"}},
				Volumes:       []api.Volume{{Name: "gcepd", VolumeSource: api.VolumeSource{GCEPersistentDisk: &api.GCEPersistentDiskVolumeSource{PDName: "my-PD", FSType: "ext4", Partition: 1, ReadOnly: false}}}},
			},
		},
	}
	invalidLabels := map[string]string{"NoUppercaseOrSpecialCharsLike=Equals": "b"}
	invalidPodTemplate := api.PodTemplate{
		Template: api.PodTemplateSpec{
			Spec: api.PodSpec{
				RestartPolicy: api.RestartPolicyAlways,
				DNSPolicy:     api.DNSClusterFirst,
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: invalidLabels,
			},
		},
	}
	type psUpdateTest struct {
		old    apps.StatefulSet
		update apps.StatefulSet
	}
	successCases := []psUpdateTest{
		{
			old: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					PodManagementPolicy: apps.OrderedReadyPodManagement,
					Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
					Template:            validPodTemplate.Template,
				},
			},
			update: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					PodManagementPolicy: apps.OrderedReadyPodManagement,
					Replicas:            3,
					Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
					Template:            validPodTemplate.Template,
				},
			},
		},
	}
	for _, successCase := range successCases {
		successCase.old.ObjectMeta.ResourceVersion = "1"
		successCase.update.ObjectMeta.ResourceVersion = "1"
		if errs := ValidateStatefulSetUpdate(&successCase.update, &successCase.old); len(errs) != 0 {
			t.Errorf("expected success: %v", errs)
		}
	}
	errorCases := map[string]psUpdateTest{
		"more than one read/write": {
			old: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: validPodTemplate.Template,
				},
			},
			update: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					PodManagementPolicy: apps.OrderedReadyPodManagement,
					Replicas:            2,
					Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
					Template:            readWriteVolumePodTemplate.Template,
				},
			},
		},
		"empty pod creation policy": {
			old: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: validPodTemplate.Template,
				},
			},
			update: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					Replicas: 3,
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: validPodTemplate.Template,
				},
			},
		},
		"invalid pod creation policy": {
			old: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: validPodTemplate.Template,
				},
			},
			update: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					PodManagementPolicy: apps.PodManagementPolicyType("Other"),
					Replicas:            3,
					Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
					Template:            validPodTemplate.Template,
				},
			},
		},
		"updates to a field other than spec.Replicas": {
			old: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: validPodTemplate.Template,
				},
			},
			update: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					PodManagementPolicy: apps.OrderedReadyPodManagement,
					Replicas:            1,
					Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
					Template:            readWriteVolumePodTemplate.Template,
				},
			},
		},
		"invalid selector": {
			old: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: validPodTemplate.Template,
				},
			},
			update: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					PodManagementPolicy: apps.OrderedReadyPodManagement,
					Replicas:            2,
					Selector:            &metav1.LabelSelector{MatchLabels: invalidLabels},
					Template:            validPodTemplate.Template,
				},
			},
		},
		"invalid pod": {
			old: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					PodManagementPolicy: apps.OrderedReadyPodManagement,
					Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
					Template:            validPodTemplate.Template,
				},
			},
			update: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					Replicas: 2,
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: invalidPodTemplate.Template,
				},
			},
		},
		"negative replicas": {
			old: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					Selector: &metav1.LabelSelector{MatchLabels: validLabels},
					Template: validPodTemplate.Template,
				},
			},
			update: apps.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
				Spec: apps.StatefulSetSpec{
					PodManagementPolicy: apps.OrderedReadyPodManagement,
					Replicas:            -1,
					Selector:            &metav1.LabelSelector{MatchLabels: validLabels},
					Template:            validPodTemplate.Template,
				},
			},
		},
	}
	for testName, errorCase := range errorCases {
		if errs := ValidateStatefulSetUpdate(&errorCase.update, &errorCase.old); len(errs) == 0 {
			t.Errorf("expected failure: %s", testName)
		}
	}
}

func TestValidateControllerRevision(t *testing.T) {
	newControllerRevision := func(name, namespace string, data runtime.Object, revision int64) apps.ControllerRevision {
		return apps.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data:     data,
			Revision: revision,
		}
	}

	ss := apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
		Spec: apps.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
			Template: api.PodTemplateSpec{
				Spec: api.PodSpec{
					RestartPolicy: api.RestartPolicyAlways,
					DNSPolicy:     api.DNSClusterFirst,
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"foo": "bar"},
				},
			},
		},
	}

	var (
		valid       = newControllerRevision("validname", "validns", &ss, 0)
		badRevision = newControllerRevision("validname", "validns", &ss, -1)
		emptyName   = newControllerRevision("", "validns", &ss, 0)
		invalidName = newControllerRevision("NoUppercaseOrSpecialCharsLike=Equals", "validns", &ss, 0)
		emptyNs     = newControllerRevision("validname", "", &ss, 100)
		invalidNs   = newControllerRevision("validname", "NoUppercaseOrSpecialCharsLike=Equals", &ss, 100)
		nilData     = newControllerRevision("validname", "NoUppercaseOrSpecialCharsLike=Equals", nil, 100)
	)

	tests := map[string]struct {
		history apps.ControllerRevision
		isValid bool
	}{
		"valid":             {valid, true},
		"negative revision": {badRevision, false},
		"empty name":        {emptyName, false},
		"invalid name":      {invalidName, false},
		"empty namespace":   {emptyNs, false},
		"invalid namespace": {invalidNs, false},
		"nil data":          {nilData, false},
	}

	for name, tc := range tests {
		errs := ValidateControllerRevision(&tc.history)
		if tc.isValid && len(errs) > 0 {
			t.Errorf("%v: unexpected error: %v", name, errs)
		}
		if !tc.isValid && len(errs) == 0 {
			t.Errorf("%v: unexpected non-error", name)
		}
	}
}

func TestValidateControllerRevisionUpdate(t *testing.T) {
	newControllerRevision := func(version, name, namespace string, data runtime.Object, revision int64) apps.ControllerRevision {
		return apps.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name:            name,
				Namespace:       namespace,
				ResourceVersion: version,
			},
			Data:     data,
			Revision: revision,
		}
	}

	ss := apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: metav1.NamespaceDefault},
		Spec: apps.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
			Template: api.PodTemplateSpec{
				Spec: api.PodSpec{
					RestartPolicy: api.RestartPolicyAlways,
					DNSPolicy:     api.DNSClusterFirst,
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"foo": "bar"},
				},
			},
		},
	}
	modifiedss := apps.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "cdf", Namespace: metav1.NamespaceDefault},
		Spec: apps.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foo": "bar"}},
			Template: api.PodTemplateSpec{
				Spec: api.PodSpec{
					RestartPolicy: api.RestartPolicyAlways,
					DNSPolicy:     api.DNSClusterFirst,
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"foo": "bar"},
				},
			},
		},
	}

	var (
		valid           = newControllerRevision("1", "validname", "validns", &ss, 0)
		noVersion       = newControllerRevision("", "validname", "validns", &ss, 0)
		changedData     = newControllerRevision("1", "validname", "validns", &modifiedss, 0)
		changedRevision = newControllerRevision("1", "validname", "validns", &ss, 1)
	)

	cases := []struct {
		name       string
		newHistory apps.ControllerRevision
		oldHistory apps.ControllerRevision
		isValid    bool
	}{
		{
			name:       "valid",
			newHistory: valid,
			oldHistory: valid,
			isValid:    true,
		},
		{
			name:       "invalid",
			newHistory: noVersion,
			oldHistory: valid,
			isValid:    false,
		},
		{
			name:       "changed data",
			newHistory: changedData,
			oldHistory: valid,
			isValid:    false,
		},
		{
			name:       "changed revision",
			newHistory: changedRevision,
			oldHistory: valid,
			isValid:    true,
		},
	}

	for _, tc := range cases {
		errs := ValidateControllerRevisionUpdate(&tc.newHistory, &tc.oldHistory)
		if tc.isValid && len(errs) > 0 {
			t.Errorf("%v: unexpected error: %v", tc.name, errs)
		}
		if !tc.isValid && len(errs) == 0 {
			t.Errorf("%v: unexpected non-error", tc.name)
		}
	}
}
