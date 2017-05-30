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

package policy

import (
	"fmt"
	"io/ioutil"

	"k8s.io/apimachinery/pkg/runtime"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	auditv1alpha1 "k8s.io/apiserver/pkg/apis/audit/v1alpha1"
	"k8s.io/apiserver/pkg/apis/audit/validation"
	"k8s.io/apiserver/pkg/audit"
)

func LoadPolicyFromFile(filePath string) (*auditinternal.Policy, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path not specified")
	}
	policyDef, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file path %q: %+v", filePath, err)
	}
	if len(policyDef) == 0 {
		return nil, fmt.Errorf("file %q was empty", filePath)
	}
	policyVersioned := &auditv1alpha1.Policy{}

	decoder := audit.Codecs.UniversalDecoder(auditv1alpha1.SchemeGroupVersion)
	if err := runtime.DecodeInto(decoder, policyDef, policyVersioned); err != nil {
		return nil, fmt.Errorf("failed decoding file %q: %v", filePath, err)
	}

	policy := &auditinternal.Policy{}
	if err := audit.Scheme.Convert(policyVersioned, policy, nil); err != nil {
		return nil, fmt.Errorf("failed converting policy: %v", err)
	}

	if err := validation.ValidatePolicy(policy); err != nil {
		return nil, err.ToAggregate()
	}
	return policy, nil
}
