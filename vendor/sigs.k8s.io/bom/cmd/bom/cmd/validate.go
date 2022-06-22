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
	"os"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
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
						return errors.Wrapf(err, "checking argument %d", i)
					}
					defer file.Close()
					fileInfo, err := file.Stat()
					if err != nil {
						return errors.Wrapf(err, "calling stat on argument %d", i)
					}
					if fileInfo.IsDir() {
						return errors.Errorf(
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
					return errors.Errorf("the path specified at %s does not exist", arg)
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
}

// Validate verify options consistency
func (opts *validateOptions) Validate() error {
	if len(opts.files) == 0 {
		return errors.New("please provide at least one artifact to validate")
	}

	return nil
}

func validateArtifacts(opts validateOptions) error {
	if err := opts.Validate(); err != nil {
		return errors.Wrap(err, "validating command line options")
	}

	if !opts.exitCode {
		logrus.Info("Checking files against SPDX Bill of Materials")
	}
	doc, err := spdx.OpenDoc(opts.sbomPath)
	if err != nil {
		return errors.Wrap(err, "opening doc")
	}

	res, err := doc.ValidateFiles(opts.files)
	if err != nil {
		return errors.Wrap(err, "validating files")
	}

	data := [][]string{}
	for _, res := range res {
		// If we only want an exit code abort on the first failure
		if !res.Success && opts.exitCode {
			logrus.Errorf("Checking %s: %s", res.FileName, res.Message)
			os.Exit(1)
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

	// Exit now if we only want the exit code
	if opts.exitCode {
		logrus.Info("All files valid")
		os.Exit(0)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"FileName", "Valid", "Message", "Invalid Hashes"})

	for _, v := range data {
		table.Append(v)
	}
	table.Render()
	return nil
}
