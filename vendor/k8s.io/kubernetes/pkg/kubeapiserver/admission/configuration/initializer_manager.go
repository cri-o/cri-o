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
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/apis/admissionregistration/v1alpha1"
)

type InitializerConfigurationLister interface {
	List(opts metav1.ListOptions) (*v1alpha1.InitializerConfigurationList, error)
}

type InitializerConfigurationManager struct {
	*poller
}

// Initializers returns the merged InitializerConfiguration.
func (im *InitializerConfigurationManager) Initializers() (*v1alpha1.InitializerConfiguration, error) {
	configuration, err := im.poller.configuration()
	if err != nil {
		return nil, err
	}
	initializerConfiguration, ok := configuration.(*v1alpha1.InitializerConfiguration)
	if !ok {
		return nil, fmt.Errorf("expected type %v, got type %v", reflect.TypeOf(initializerConfiguration), reflect.TypeOf(configuration))
	}
	return initializerConfiguration, nil
}

func NewInitializerConfigurationManager(c InitializerConfigurationLister) *InitializerConfigurationManager {
	getFn := func() (runtime.Object, error) {
		list, err := c.List(metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		return mergeInitializerConfigurations(list), nil
	}
	return &InitializerConfigurationManager{
		newPoller(getFn)}
}

func mergeInitializerConfigurations(initializerConfigurationList *v1alpha1.InitializerConfigurationList) *v1alpha1.InitializerConfiguration {
	configurations := initializerConfigurationList.Items
	sort.SliceStable(configurations, InitializerConfigurationSorter(configurations).ByName)
	var ret v1alpha1.InitializerConfiguration
	for _, c := range configurations {
		ret.Initializers = append(ret.Initializers, c.Initializers...)
	}
	return &ret
}

type InitializerConfigurationSorter []v1alpha1.InitializerConfiguration

func (a InitializerConfigurationSorter) ByName(i, j int) bool {
	return a[i].Name < a[j].Name
}
