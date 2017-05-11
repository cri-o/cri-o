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

// this file contains factories with no other dependencies

package util

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/kubectl/resource"
	"k8s.io/kubernetes/pkg/printers"
)

type ring2Factory struct {
	clientAccessFactory  ClientAccessFactory
	objectMappingFactory ObjectMappingFactory
}

func NewBuilderFactory(clientAccessFactory ClientAccessFactory, objectMappingFactory ObjectMappingFactory) BuilderFactory {
	f := &ring2Factory{
		clientAccessFactory:  clientAccessFactory,
		objectMappingFactory: objectMappingFactory,
	}

	return f
}

func (f *ring2Factory) PrinterForCommand(cmd *cobra.Command) (printers.ResourcePrinter, bool, error) {
	mapper, typer := f.objectMappingFactory.Object()
	// TODO: used by the custom column implementation and the name implementation, break this dependency
	decoders := []runtime.Decoder{f.clientAccessFactory.Decoder(true), unstructured.UnstructuredJSONScheme}
	return PrinterForCommand(cmd, mapper, typer, decoders)
}

func (f *ring2Factory) PrinterForMapping(cmd *cobra.Command, mapping *meta.RESTMapping, withNamespace bool) (printers.ResourcePrinter, error) {
	printer, generic, err := f.PrinterForCommand(cmd)
	if err != nil {
		return nil, err
	}

	// Make sure we output versioned data for generic printers
	if generic {
		if mapping == nil {
			return nil, fmt.Errorf("no serialization format found")
		}
		version := mapping.GroupVersionKind.GroupVersion()
		if version.Empty() {
			return nil, fmt.Errorf("no serialization format found")
		}

		printer = printers.NewVersionedPrinter(printer, mapping.ObjectConvertor, version, mapping.GroupVersionKind.GroupVersion())
	} else {
		// Some callers do not have "label-columns" so we can't use the GetFlagStringSlice() helper
		columnLabel, err := cmd.Flags().GetStringSlice("label-columns")
		if err != nil {
			columnLabel = []string{}
		}
		printer, err = f.clientAccessFactory.Printer(mapping, printers.PrintOptions{
			NoHeaders:          GetFlagBool(cmd, "no-headers"),
			WithNamespace:      withNamespace,
			Wide:               GetWideFlag(cmd),
			ShowAll:            GetFlagBool(cmd, "show-all"),
			ShowLabels:         GetFlagBool(cmd, "show-labels"),
			AbsoluteTimestamps: isWatch(cmd),
			ColumnLabels:       columnLabel,
		})
		if err != nil {
			return nil, err
		}
		printer = maybeWrapSortingPrinter(cmd, printer)
	}

	return printer, nil
}

func (f *ring2Factory) PrintObject(cmd *cobra.Command, mapper meta.RESTMapper, obj runtime.Object, out io.Writer) error {
	// try to get a typed object
	_, typer := f.objectMappingFactory.Object()
	gvks, _, err := typer.ObjectKinds(obj)

	// fall back to an unstructured object if we get something unregistered
	if runtime.IsNotRegisteredError(err) {
		_, typer, unstructuredErr := f.objectMappingFactory.UnstructuredObject()
		if unstructuredErr != nil {
			// if we can't get an unstructured typer, return the original error
			return err
		}
		gvks, _, err = typer.ObjectKinds(obj)
	}

	if err != nil {
		return err
	}

	mapping, err := mapper.RESTMapping(gvks[0].GroupKind())
	if err != nil {
		return err
	}

	printer, err := f.PrinterForMapping(cmd, mapping, false)
	if err != nil {
		return err
	}
	return printer.PrintObj(obj, out)
}

func (f *ring2Factory) NewBuilder() *resource.Builder {
	mapper, typer := f.objectMappingFactory.Object()

	return resource.NewBuilder(mapper, typer, resource.ClientMapperFunc(f.objectMappingFactory.ClientForMapping), f.clientAccessFactory.Decoder(true))
}
