/*
Copyright 2021 The Kubernetes Authors.

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
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"sigs.k8s.io/release-utils/log"
	"sigs.k8s.io/release-utils/version"
)

var rootCmd = &cobra.Command{
	Short: "A tool for working with SPDX manifests",
	Long: `bom (Bill of Materials)

bom is a little utility that lets software authors generate
SPDX manifests to describe the contents of a release. The
SPDX manifests provide a way to list and verify all items
contained in packages, images, and individual files while
packing the data along with licensing information.

bom is still in its early stages and it is an effort to open
the libraries developed for the Kubernetes SBOM for other
projects to use.

`,
	Use:               "bom",
	SilenceUsage:      false,
	PersistentPreRunE: initLogging,
}

type commandLineOptions struct {
	logLevel string
}

var commandLineOpts = &commandLineOptions{}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&commandLineOpts.logLevel,
		"log-level",
		"info",
		fmt.Sprintf("the logging verbosity, either %s", log.LevelNames()),
	)

	AddGenerate(rootCmd)
	AddDocument(rootCmd)
	AddValidate(rootCmd)
	rootCmd.AddCommand(version.WithFont("doom"))
}

// Execute builds the command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func initLogging(*cobra.Command, []string) error {
	return log.SetupGlobalLogger(commandLineOpts.logLevel)
}
