/*
Copyright 2022 The Kubernetes Authors.

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

package query

import (
	"errors"
	"fmt"

	"sigs.k8s.io/bom/pkg/spdx"
)

type Engine struct {
	impl     engineImplementation
	Document *spdx.Document
	MaxDepth int
}

func New() *Engine {
	return &Engine{
		impl: &defaultEngineImplementation{},
	}
}

// Open reads a document from the specified path
func (e *Engine) Open(path string) error {
	doc, err := spdx.OpenDoc(path)
	if err != nil {
		return fmt.Errorf("opening doc: %w", err)
	}
	e.Document = doc
	return nil
}

// Query takes an expression as a string and filters the loaded document
func (e *Engine) Query(expString string) (fr FilterResults, err error) {
	if e.Document == nil {
		return fr, errors.New("query engine has no document open")
	}

	exp, err := NewExpression(expString)
	if err != nil {
		return fr, fmt.Errorf("reading expression: %w", err)
	}

	resultSet := e.impl.resultsFromDocument(e.Document)

	for _, filter := range exp.Filters {
		resultSet = *resultSet.Apply(filter)
	}

	return resultSet, nil
}

type engineImplementation interface {
	resultsFromDocument(*spdx.Document) FilterResults
}

type defaultEngineImplementation struct{}

func (di *defaultEngineImplementation) resultsFromDocument(doc *spdx.Document) FilterResults {
	fr := FilterResults{
		Objects: map[string]spdx.Object{},
	}
	for _, p := range doc.Packages {
		if p.SPDXID() == "" {
			continue
		}
		fr.Objects[p.SPDXID()] = p
	}
	for _, f := range doc.Files {
		if f.SPDXID() == "" {
			continue
		}
		fr.Objects[f.SPDXID()] = f
	}
	return fr
}
