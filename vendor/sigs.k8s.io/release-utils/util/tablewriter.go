/*
Copyright 2025 The Kubernetes Authors.

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

package util

import (
	"io"

	"github.com/olekukonko/tablewriter"
)

// NewTableWriter creates a new table writer with the given output and options.
func NewTableWriter(output io.Writer, options ...tablewriter.Option) *tablewriter.Table {
	table := tablewriter.NewWriter(output)
	for _, opt := range options {
		table.Options(opt)
	}

	return table
}
