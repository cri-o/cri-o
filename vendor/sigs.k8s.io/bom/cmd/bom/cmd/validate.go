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

package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"sigs.k8s.io/bom/pkg/spdx"
	"sigs.k8s.io/release-utils/util"
)

func AddValidate(parent *cobra.Command) {
	valOpts := validateOptions{
		files: []string{},
	}

	cmd := &cobra.Command{
		Short: "bom validate → Check artifacts against an sbom",
		Long: `bom validate → Check artifacts against an sbom

validate is the bom subcommand to check artifacts against SPDX
manifests.

This is an experimental command. The first iteration has support
for checking files.

`,
		Use:               "validate",
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: initLogging,

		RunE: func(cmd *cobra.Command, args []string) error {
			for i, arg := range args {
				if util.Exists(arg) {
					file, err := os.Open(arg)
					if err != nil {
						return fmt.Errorf("checking argument %d: %w", i, err)
					}
					defer file.Close()
					fileInfo, err := file.Stat()
					if err != nil {
						return fmt.Errorf("calling stat on argument %d: %w", i, err)
					}
					if fileInfo.IsDir() {
						return fmt.Errorf(
							"the path %s is a directory, only files are supported at this time",
							file.Name(),
						)
					}
					if i == 0 {
						valOpts.sbomPath = arg
					} else {
						valOpts.files = append(valOpts.files, file.Name())
					}
				} else {
					return fmt.Errorf("the path specified at %s does not exist", arg)
				}
			}
			return validateArtifacts(valOpts)
		},
	}

	cmd.PersistentFlags().StringSliceVarP(
		&valOpts.files,
		"files",
		"f",
		[]string{},
		"list of files to verify",
	)

	cmd.PersistentFlags().StringVarP(
		&valOpts.dir,
		"dir",
		"d",
		"",
		"a whole directory to verify",
	)

	cmd.PersistentFlags().BoolVarP(
		&valOpts.exitCode,
		"exit-code",
		"e",
		false,
		"when true, bom will exit with exit code 1 if invalid artifacts are found",
	)

	parent.AddCommand(cmd)
}

type validateOptions struct {
	exitCode bool
	sbomPath string
	files    []string
	dir      string
}

// Validate verify options consistency
func (opts *validateOptions) Validate() error {
	if len(opts.files) == 0 && opts.dir == "" {
		return errors.New("please provide at least one artifact file or directory to validate")
	}

	return nil
}

func validateArtifacts(opts validateOptions) error {
	if err := opts.Validate(); err != nil {
		return fmt.Errorf("validating command line options: %w", err)
	}

	if !opts.exitCode {
		logrus.Info("Checking files against SPDX Bill of Materials")
	}
	doc, err := spdx.OpenDoc(opts.sbomPath)
	if err != nil {
		return fmt.Errorf("opening doc: %w", err)
	}

	files := []string{}
	if opts.dir != "" {
		if err := os.Chdir(opts.dir); err != nil {
			return fmt.Errorf("unable to change to dir %s: %w", opts.dir, err)
		}

		if err := filepath.Walk(".",
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if info.IsDir() {
					return nil
				}

				files = append(files, path)
				return nil
			},
		); err != nil {
			return fmt.Errorf("unable to walk current dir: %w", err)
		}
	}
	files = append(files, opts.files...)

	res, err := doc.ValidateFiles(files)
	if err != nil {
		return fmt.Errorf("validating files: %w", err)
	}

	data := [][]string{}
	errored := false
	for _, res := range res {
		// If we only want an exit code abort on the first failure
		if !res.Success && opts.exitCode {
			errored = true
		}
		resRow := []string{
			res.FileName,
			"FAIL",
			res.Message,
			"-",
		}
		if res.Message == spdx.MessageHashMismatch && len(res.FailedAlgorithms) > 0 {
			resRow[3] = strings.Join(res.FailedAlgorithms, " ")
		}
		if res.Success {
			resRow[1] = "OK"
		}
		data = append(data, resRow)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"FileName", "Valid", "Message", "Invalid Hashes"})

	for _, v := range data {
		table.Append(v)
	}
	table.Render()

	if errored {
		return errors.New("failed to validate all files")
	}

	return nil
}
