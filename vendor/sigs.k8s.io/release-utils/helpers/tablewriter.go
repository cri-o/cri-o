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

package helpers

import (
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

// NewTableWriter creates a new table writer with the given output and options.
func NewTableWriter(output io.Writer, options ...tablewriter.Option) *tablewriter.Table {
	table := tablewriter.NewWriter(output)
	for _, opt := range options {
		table.Options(opt)
	}

	return table
}

// NewTableWriterWithDefaults creates a new table writer with default markdown configuration.
// It includes left alignment, markdown renderer, and custom borders optimized for terminal output.
func NewTableWriterWithDefaults(output io.Writer, options ...tablewriter.Option) *tablewriter.Table {
	defaultOptions := []tablewriter.Option{
		tablewriter.WithConfig(tablewriter.Config{
			Header: tw.CellConfig{
				Alignment: tw.CellAlignment{Global: tw.AlignLeft},
			},
		}),
		tablewriter.WithRenderer(renderer.NewMarkdown()),
		tablewriter.WithRendition(tw.Rendition{
			Symbols: tw.NewSymbols(tw.StyleMarkdown),
			Borders: tw.Border{
				Left:   tw.On,
				Top:    tw.Off,
				Right:  tw.On,
				Bottom: tw.Off,
			},
			Settings: tw.Settings{
				Separators: tw.Separators{
					BetweenRows: tw.On,
				},
			},
		}),
		tablewriter.WithRowAutoWrap(tw.WrapNone),
	}

	defaultOptions = append(defaultOptions, options...)

	return NewTableWriter(output, defaultOptions...)
}

// NewTableWriterWithDefaultsAndHeader creates a new table writer with default configuration and header.
func NewTableWriterWithDefaultsAndHeader(output io.Writer, header []string, options ...tablewriter.Option) *tablewriter.Table {
	headerOption := tablewriter.WithHeader(header)
	allOptions := append([]tablewriter.Option{headerOption}, options...)

	return NewTableWriterWithDefaults(output, allOptions...)
}
