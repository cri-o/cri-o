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

package fake

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
	admissionregistration "k8s.io/kubernetes/pkg/apis/admissionregistration"
)

// FakeInitializerConfigurations implements InitializerConfigurationInterface
type FakeInitializerConfigurations struct {
	Fake *FakeAdmissionregistration
	ns   string
}

var initializerconfigurationsResource = schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "", Resource: "initializerconfigurations"}

var initializerconfigurationsKind = schema.GroupVersionKind{Group: "admissionregistration.k8s.io", Version: "", Kind: "InitializerConfiguration"}

func (c *FakeInitializerConfigurations) Create(initializerConfiguration *admissionregistration.InitializerConfiguration) (result *admissionregistration.InitializerConfiguration, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(initializerconfigurationsResource, c.ns, initializerConfiguration), &admissionregistration.InitializerConfiguration{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admissionregistration.InitializerConfiguration), err
}

func (c *FakeInitializerConfigurations) Update(initializerConfiguration *admissionregistration.InitializerConfiguration) (result *admissionregistration.InitializerConfiguration, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(initializerconfigurationsResource, c.ns, initializerConfiguration), &admissionregistration.InitializerConfiguration{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admissionregistration.InitializerConfiguration), err
}

func (c *FakeInitializerConfigurations) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(initializerconfigurationsResource, c.ns, name), &admissionregistration.InitializerConfiguration{})

	return err
}

func (c *FakeInitializerConfigurations) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(initializerconfigurationsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &admissionregistration.InitializerConfigurationList{})
	return err
}

func (c *FakeInitializerConfigurations) Get(name string, options v1.GetOptions) (result *admissionregistration.InitializerConfiguration, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(initializerconfigurationsResource, c.ns, name), &admissionregistration.InitializerConfiguration{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admissionregistration.InitializerConfiguration), err
}

func (c *FakeInitializerConfigurations) List(opts v1.ListOptions) (result *admissionregistration.InitializerConfigurationList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(initializerconfigurationsResource, initializerconfigurationsKind, c.ns, opts), &admissionregistration.InitializerConfigurationList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &admissionregistration.InitializerConfigurationList{}
	for _, item := range obj.(*admissionregistration.InitializerConfigurationList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested initializerConfigurations.
func (c *FakeInitializerConfigurations) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(initializerconfigurationsResource, c.ns, opts))

}

// Patch applies the patch and returns the patched initializerConfiguration.
func (c *FakeInitializerConfigurations) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *admissionregistration.InitializerConfiguration, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(initializerconfigurationsResource, c.ns, name, data, subresources...), &admissionregistration.InitializerConfiguration{})

	if obj == nil {
		return nil, err
	}
	return obj.(*admissionregistration.InitializerConfiguration), err
}
