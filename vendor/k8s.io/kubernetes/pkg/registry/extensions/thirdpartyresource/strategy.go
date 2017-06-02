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

package thirdpartyresource

import (
	"fmt"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/apis/extensions/validation"
)

// strategy implements behavior for ThirdPartyResource objects
type strategy struct {
	runtime.ObjectTyper
}

// Strategy is the default logic that applies when creating and updating ThirdPartyResource
// objects via the REST API.
var Strategy = strategy{api.Scheme}

var _ = rest.RESTCreateStrategy(Strategy)

var _ = rest.RESTUpdateStrategy(Strategy)

func (strategy) NamespaceScoped() bool {
	return false
}

func (strategy) GenerateName(base string) string {
	return ""
}

func (strategy) PrepareForCreate(ctx genericapirequest.Context, obj runtime.Object) {
}

func (strategy) Validate(ctx genericapirequest.Context, obj runtime.Object) field.ErrorList {
	return validation.ValidateThirdPartyResource(obj.(*extensions.ThirdPartyResource))
}

// Canonicalize normalizes the object after validation.
func (strategy) Canonicalize(obj runtime.Object) {
}

func (strategy) AllowCreateOnUpdate() bool {
	return false
}

func (strategy) PrepareForUpdate(ctx genericapirequest.Context, obj, old runtime.Object) {
}

func (strategy) ValidateUpdate(ctx genericapirequest.Context, obj, old runtime.Object) field.ErrorList {
	return validation.ValidateThirdPartyResourceUpdate(obj.(*extensions.ThirdPartyResource), old.(*extensions.ThirdPartyResource))
}

func (strategy) AllowUnconditionalUpdate() bool {
	return true
}

// GetAttrs returns labels and fields of a given object for filtering purposes.
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	tpr, ok := obj.(*extensions.ThirdPartyResource)
	if !ok {
		return nil, nil, fmt.Errorf("not a ThirdPartyResource")
	}
	return labels.Set(tpr.Labels), SelectableFields(tpr), nil
}

// Matcher returns a generic matcher for a given label and field selector.
func Matcher(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}

// SelectableFields returns a field set that can be used for filter selection
func SelectableFields(obj *extensions.ThirdPartyResource) fields.Set {
	return nil
}
