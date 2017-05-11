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

package kubectl

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api/v1"
	appsv1beta1 "k8s.io/kubernetes/pkg/apis/apps/v1beta1"
	extensionsv1beta1 "k8s.io/kubernetes/pkg/apis/extensions/v1beta1"
)

// DeploymentBasicGeneratorV1 supports stable generation of a deployment
type DeploymentBasicGeneratorV1 struct {
	Name   string
	Images []string
}

// Ensure it supports the generator pattern that uses parameters specified during construction
var _ StructuredGenerator = &DeploymentBasicGeneratorV1{}

func (DeploymentBasicGeneratorV1) ParamNames() []GeneratorParam {
	return []GeneratorParam{
		{"name", true},
		{"image", true},
	}
}

func (s DeploymentBasicGeneratorV1) Generate(params map[string]interface{}) (runtime.Object, error) {
	err := ValidateParams(s.ParamNames(), params)
	if err != nil {
		return nil, err
	}
	name, isString := params["name"].(string)
	if !isString {
		return nil, fmt.Errorf("expected string, saw %v for 'name'", name)
	}
	imageStrings, isArray := params["image"].([]string)
	if !isArray {
		return nil, fmt.Errorf("expected []string, found :%v", imageStrings)
	}
	delegate := &DeploymentBasicGeneratorV1{Name: name, Images: imageStrings}
	return delegate.StructuredGenerate()
}

// StructuredGenerate outputs a deployment object using the configured fields
func (s *DeploymentBasicGeneratorV1) StructuredGenerate() (runtime.Object, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}

	podSpec := v1.PodSpec{Containers: []v1.Container{}}
	for _, imageString := range s.Images {
		// Retain just the image name
		imageSplit := strings.Split(imageString, "/")
		name := imageSplit[len(imageSplit)-1]
		// Remove any tag or hash
		if strings.Contains(name, ":") {
			name = strings.Split(name, ":")[0]
		} else if strings.Contains(name, "@") {
			name = strings.Split(name, "@")[0]
		}
		podSpec.Containers = append(podSpec.Containers, v1.Container{Name: name, Image: imageString})
	}

	// setup default label and selector
	labels := map[string]string{}
	labels["app"] = s.Name
	one := int32(1)
	selector := metav1.LabelSelector{MatchLabels: labels}
	deployment := extensionsv1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   s.Name,
			Labels: labels,
		},
		Spec: extensionsv1beta1.DeploymentSpec{
			Replicas: &one,
			Selector: &selector,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}
	return &deployment, nil
}

// validate validates required fields are set to support structured generation
func (s *DeploymentBasicGeneratorV1) validate() error {
	if len(s.Name) == 0 {
		return fmt.Errorf("name must be specified")
	}
	if len(s.Images) == 0 {
		return fmt.Errorf("at least one image must be specified")
	}
	return nil
}

// DeploymentBasicAppsGeneratorV1 supports stable generation of a deployment under apps/v1beta1 endpoint
type DeploymentBasicAppsGeneratorV1 struct {
	Name   string
	Images []string
}

// Ensure it supports the generator pattern that uses parameters specified during construction
var _ StructuredGenerator = &DeploymentBasicAppsGeneratorV1{}

func (DeploymentBasicAppsGeneratorV1) ParamNames() []GeneratorParam {
	return []GeneratorParam{
		{"name", true},
		{"image", true},
	}
}

func (s DeploymentBasicAppsGeneratorV1) Generate(params map[string]interface{}) (runtime.Object, error) {
	err := ValidateParams(s.ParamNames(), params)
	if err != nil {
		return nil, err
	}
	name, isString := params["name"].(string)
	if !isString {
		return nil, fmt.Errorf("expected string, saw %v for 'name'", name)
	}
	imageStrings, isArray := params["image"].([]string)
	if !isArray {
		return nil, fmt.Errorf("expected []string, found :%v", imageStrings)
	}
	delegate := &DeploymentBasicAppsGeneratorV1{Name: name, Images: imageStrings}
	return delegate.StructuredGenerate()
}

// StructuredGenerate outputs a deployment object using the configured fields
func (s *DeploymentBasicAppsGeneratorV1) StructuredGenerate() (runtime.Object, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}

	podSpec := v1.PodSpec{Containers: []v1.Container{}}
	for _, imageString := range s.Images {
		// Retain just the image name
		imageSplit := strings.Split(imageString, "/")
		name := imageSplit[len(imageSplit)-1]
		// Remove any tag or hash
		if strings.Contains(name, ":") {
			name = strings.Split(name, ":")[0]
		} else if strings.Contains(name, "@") {
			name = strings.Split(name, "@")[0]
		}
		podSpec.Containers = append(podSpec.Containers, v1.Container{Name: name, Image: imageString})
	}

	// setup default label and selector
	labels := map[string]string{}
	labels["app"] = s.Name
	one := int32(1)
	selector := metav1.LabelSelector{MatchLabels: labels}
	deployment := appsv1beta1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:   s.Name,
			Labels: labels,
		},
		Spec: appsv1beta1.DeploymentSpec{
			Replicas: &one,
			Selector: &selector,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}
	return &deployment, nil
}

// validate validates required fields are set to support structured generation
func (s *DeploymentBasicAppsGeneratorV1) validate() error {
	if len(s.Name) == 0 {
		return fmt.Errorf("name must be specified")
	}
	if len(s.Images) == 0 {
		return fmt.Errorf("at least one image must be specified")
	}
	return nil
}
