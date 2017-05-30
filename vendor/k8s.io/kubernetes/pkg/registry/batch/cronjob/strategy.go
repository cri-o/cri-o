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

package cronjob

import (
	"fmt"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/names"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/apis/batch"
	"k8s.io/kubernetes/pkg/apis/batch/validation"
)

// cronJobStrategy implements verification logic for Replication Controllers.
type cronJobStrategy struct {
	runtime.ObjectTyper
	names.NameGenerator
}

// Strategy is the default logic that applies when creating and updating CronJob objects.
var Strategy = cronJobStrategy{api.Scheme, names.SimpleNameGenerator}

// DefaultGarbageCollectionPolicy returns Orphan because that was the default
// behavior before the server-side garbage collection was implemented.
func (cronJobStrategy) DefaultGarbageCollectionPolicy() rest.GarbageCollectionPolicy {
	return rest.OrphanDependents
}

// NamespaceScoped returns true because all scheduled jobs need to be within a namespace.
func (cronJobStrategy) NamespaceScoped() bool {
	return true
}

// PrepareForCreate clears the status of a scheduled job before creation.
func (cronJobStrategy) PrepareForCreate(ctx genericapirequest.Context, obj runtime.Object) {
	cronJob := obj.(*batch.CronJob)
	cronJob.Status = batch.CronJobStatus{}
}

// PrepareForUpdate clears fields that are not allowed to be set by end users on update.
func (cronJobStrategy) PrepareForUpdate(ctx genericapirequest.Context, obj, old runtime.Object) {
	newCronJob := obj.(*batch.CronJob)
	oldCronJob := old.(*batch.CronJob)
	newCronJob.Status = oldCronJob.Status
}

// Validate validates a new scheduled job.
func (cronJobStrategy) Validate(ctx genericapirequest.Context, obj runtime.Object) field.ErrorList {
	cronJob := obj.(*batch.CronJob)
	return validation.ValidateCronJob(cronJob)
}

// Canonicalize normalizes the object after validation.
func (cronJobStrategy) Canonicalize(obj runtime.Object) {
}

func (cronJobStrategy) AllowUnconditionalUpdate() bool {
	return true
}

// AllowCreateOnUpdate is false for scheduled jobs; this means a POST is needed to create one.
func (cronJobStrategy) AllowCreateOnUpdate() bool {
	return false
}

// ValidateUpdate is the default update validation for an end user.
func (cronJobStrategy) ValidateUpdate(ctx genericapirequest.Context, obj, old runtime.Object) field.ErrorList {
	return validation.ValidateCronJob(obj.(*batch.CronJob))
}

type cronJobStatusStrategy struct {
	cronJobStrategy
}

var StatusStrategy = cronJobStatusStrategy{Strategy}

func (cronJobStatusStrategy) PrepareForUpdate(ctx genericapirequest.Context, obj, old runtime.Object) {
	newJob := obj.(*batch.CronJob)
	oldJob := old.(*batch.CronJob)
	newJob.Spec = oldJob.Spec
}

func (cronJobStatusStrategy) ValidateUpdate(ctx genericapirequest.Context, obj, old runtime.Object) field.ErrorList {
	return field.ErrorList{}
}

// CronJobToSelectableFields returns a field set that represents the object for matching purposes.
func CronJobToSelectableFields(cronJob *batch.CronJob) fields.Set {
	return generic.ObjectMetaFieldsSet(&cronJob.ObjectMeta, true)
}

// GetAttrs returns labels and fields of a given object for filtering purposes.
func GetAttrs(obj runtime.Object) (labels.Set, fields.Set, error) {
	cronJob, ok := obj.(*batch.CronJob)
	if !ok {
		return nil, nil, fmt.Errorf("Given object is not a scheduled job.")
	}
	return labels.Set(cronJob.ObjectMeta.Labels), CronJobToSelectableFields(cronJob), nil
}

// MatchCronJob is the filter used by the generic etcd backend to route
// watch events from etcd to clients of the apiserver only interested in specific
// labels/fields.
func MatchCronJob(label labels.Selector, field fields.Selector) storage.SelectionPredicate {
	return storage.SelectionPredicate{
		Label:    label,
		Field:    field,
		GetAttrs: GetAttrs,
	}
}
