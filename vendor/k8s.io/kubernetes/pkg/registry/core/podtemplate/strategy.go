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

package podtemplate

import (
	"fmt"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/validation"
)

// podTemplateStrategy implements behavior for PodTemplates
type podTemplateStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

// Strategy is the default logic that applies when creating and updating PodTemplate
// objects via the REST API.
var Strategy = podTemplateStrategy{api.Scheme, names.SimpleNameGenerator}

// NamespaceScoped is true for pod templates.
func (podTemplateStrategy) NamespaceScoped() bool {
	return true
}

// PrepareForCreate clears fields that are not allowed to be set by end users on creation.
func (podTemplateStrategy) PrepareForCreate(ctx genericapirequest.Context, obj runtime.Object) {
	_ = obj.(*api.PodTemplate)
}

// Validate validates a new pod template.
func (podTemplateStrategy) Validate(ctx genericapirequest.Context, obj runtime.Object) field.ErrorList {
	pod := obj.(*api.PodTemplate)
	return validation.ValidatePodTemplate(pod)
}

// Canonicalize normalizes the object after validation.
func (podTemplateStrategy) Canonicalize(obj runtime.Object) {
}

// AllowCreateOnUpdate is false for pod templates.
func (podTemplateStrategy) AllowCreateOnUpdate() bool {
	return false
}

// PrepareForUpdate clears fields that are not allowed to be set by end users on update.
func (podTemplateStrategy) PrepareForUpdate(ctx genericapirequest.Context, obj, old runtime.Object) {
	_ = obj.(*api.PodTemplate)
}

// ValidateUpdate is the default update validation for an end user.
func (podTemplateStrategy) ValidateUpdate(ctx genericapirequest.Context, obj, old runtime.Object) field.ErrorList {
	return validation.ValidatePodTemplateUpdate(obj.(*api.PodTemplate), old.(*api.PodTemplate))
}

func (podTemplateStrategy) AllowUnconditionalUpdate() bool {
	return true
}

func (podTemplateStrategy) Export(ctx genericapirequest.Context, obj runtime.Object, exact bool) error {
	// Do nothing
	return nil
}

func PodTemplateToSelectableFields(podTemplate *api.PodTemplate) fields.Set {
	return nil
}

// GetAttrs returns labels and fields of a given object for filtering purposes.
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	pt, ok := obj.(*api.PodTemplate)
	if !ok {
		return nil, nil, fmt.Errorf("given object is not a pod template.")
	}
	return labels.Set(pt.ObjectMeta.Labels), PodTemplateToSelectableFields(pt), nil
}

func MatchPodTemplate(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}
