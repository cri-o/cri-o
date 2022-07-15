/*
Copyright 2020 The Kubernetes Authors.

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

package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/zeitgeist/dependency"
)

type OutputFormat string

const (
	YAML OutputFormat = "yaml"
	JSON OutputFormat = "json"
	LOG  OutputFormat = "log"
)

const defaultOutputFileName = "dependencies_output"

type exportOptions struct {
	rootOpts     *options
	outputFormat string
	outputFile   string
}

func (exo *exportOptions) setAndValidate() error {
	if err := exo.rootOpts.setAndValidate(); err != nil {
		return err
	}

	if exo.rootOpts.localOnly {
		logrus.Warn("ignoring flag '--local-only'")
	}

	switch OutputFormat(exo.outputFormat) {
	case YAML:
	case JSON:
	case LOG:
	default:
		return fmt.Errorf("unsuported output format")
	}

	if exo.outputFile != "" && OutputFormat(exo.outputFormat) == LOG {
		logrus.Warnf("ignoring --output-file as --output-format is 'log'")
	}

	return nil
}

var exportOpts = &exportOptions{}

func addExport(topLevel *cobra.Command) {
	exo := exportOpts
	exo.rootOpts = rootOpts

	cmd := &cobra.Command{
		Use:           "export",
		Short:         "Export list of 'latest' upstream versions available",
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE: func(*cobra.Command, []string) error {
			if err := exo.setAndValidate(); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(exo)
		},
	}

	cmd.PersistentFlags().StringVar(
		&exportOpts.outputFormat,
		"output-format",
		"log",
		"format of the output. Supported values are 'log', 'json' and 'yaml'. If not provided it will default to printing log.",
	)

	cmd.PersistentFlags().StringVar(
		&exportOpts.outputFile,
		"output-file",
		"",
		"file to write output. Use only if --output-format is 'json' or 'yaml'. If not specified will default to dependency_output.(json|yaml).",
	)

	topLevel.AddCommand(cmd)
}

// runValidate is the function invoked by 'addValidate', responsible for
// validating dependencies in a specified configuration file.
func runExport(opts *exportOptions) error {
	client := dependency.NewClient()

	updates, err := client.RemoteExport(opts.rootOpts.configFile)
	if err != nil {
		return err
	}

	return output(opts, updates)
}

func output(opts *exportOptions, updates []dependency.VersionUpdate) error {
	if OutputFormat(opts.outputFormat) == LOG {
		return outputLog(updates)
	}
	return outputFile(opts, updates)
}

func outputLog(updates []dependency.VersionUpdate) error {
	for _, update := range updates {
		if update.Version == update.NewVersion {
			logrus.Debugf(
				"No update available for dependency %v: %v (latest: %v)\n",
				update.Name,
				update.Version,
				update.NewVersion,
			)
		} else {
			fmt.Printf(
				"Update available for dependency %v: %v (current: %v)\n",
				update.Name,
				update.NewVersion,
				update.Version,
			)
		}
	}
	return nil
}

func outputFile(opts *exportOptions, updates []dependency.VersionUpdate) error {
	var outputFile string
	if opts.outputFile != "" {
		outputFile = opts.outputFile
	} else {
		outputFile = fmt.Sprintf("%v.%v", defaultOutputFileName, opts.outputFormat)
	}

	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to open output file: %w", err)
	}
	defer f.Close()

	var b []byte
	switch OutputFormat(opts.outputFormat) {
	case YAML:
		b, err = yaml.Marshal(updates)
		if err != nil {
			return err
		}
	case JSON:
		b, err = json.Marshal(updates)
		if err != nil {
			return err
		}
	}

	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}
