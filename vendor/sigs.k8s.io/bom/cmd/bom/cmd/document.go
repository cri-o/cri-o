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
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"sigs.k8s.io/bom/pkg/spdx"
)

func AddDocument(parent *cobra.Command) {
	documentCmd := &cobra.Command{
		Short:             "bom document → Work with SPDX documents",
		Use:               "document",
		SilenceUsage:      false,
		SilenceErrors:     true,
		PersistentPreRunE: initLogging,
	}

	AddOutline(documentCmd)
	AddQuery(documentCmd)
	parent.AddCommand(documentCmd)
}

/*
	func AddQuery(parent *cobra.Command) {
		queryCmd := &cobra.Command{
			Short:             "bom document query → Query for data in an SPDX document",
			Use:               "query SPDX_FILE",
			SilenceUsage:      false,
			SilenceErrors:     false,
			PersistentPreRunE: initLogging,
			RunE: func(cmd *cobra.Command, args []string) error {
				if len(args) < 2 {
					cmd.Help()
					return nil
				}
				doc, err := spdx.OpenDoc(args[0])
				if err != nil {
					return fmt.Errorf("opening document: %w", err)
				}

				// Parse the purl
				pquery, err := purl.FromString(args[1])
				if err != nil {
					return fmt.Errorf(err, "parsing purl")
				}

				// Create the purl we will use to query
				var purlType, purlNamespace, purlName, purlVersion string
				if pquery.Type != "*" {
					purlType = pquery.Type
				}
				if pquery.Name != "*" {
					purlName = pquery.Name
				}
				if pquery.Version != "*" {
					purlVersion = pquery.Version
				}
				if pquery.Namespace != "*" {
					purlNamespace = pquery.Namespace
				}
				pSpec := purl.NewPackageURL(purlType, purlNamespace, purlName, purlVersion, purl.Qualifiers{}, "")

				pkgs := doc.GetPackagesByPurl(pSpec)
				for _, pk := range pkgs {
					fmt.Printf("%s (%s)\n", pk.Name, pk.Purl())
				}
				return nil
			},
		}
		parent.AddCommand(queryCmd)
	}
*/
func AddOutline(parent *cobra.Command) {
	outlineOpts := &spdx.DrawingOptions{}
	outlineCmd := &cobra.Command{
		PersistentPreRunE: initLogging,
		Short:             "bom document outline → Draw structure of a SPDX document",
		Long: `bom document outline → Draw structure of a SPDX document",

This subcommand draws a tree-like outline to help the user visualize
the structure of the bom. Even when an SBOM represents a graph structure,
drawing a tree helps a lot to understand what is contained in the document.

You can define a level of depth to limit the expansion of the entities.
For example set --depth=1 to only visualize only the files and packages
attached directly to the root of the document.

bom will try to add useful information to the oultine but, if needed, you can
set the --spdx-ids to only output the IDs of the entities.

`,
		Use:           "outline SPDX_FILE|URL",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				cmd.Help() //nolint:errcheck
				return errors.New("you should only specify one file")
			}
			doc, err := spdx.OpenDoc(args[0])
			if err != nil {
				return fmt.Errorf("opening doc: %w", err)
			}
			output, err := doc.Outline(outlineOpts)
			if err != nil {
				return fmt.Errorf("generating document outline: %w", err)
			}
			fmt.Println(spdx.Banner())
			fmt.Println(output)
			return nil
		},
	}
	outlineCmd.PersistentFlags().IntVarP(
		&outlineOpts.Recursion,
		"depth",
		"d",
		-1,
		"recursion level",
	)

	outlineCmd.PersistentFlags().BoolVar(
		&outlineOpts.OnlyIDs,
		"spdx-ids",
		false,
		"use SPDX identifiers in tree nodes instead of names",
	)
	outlineCmd.PersistentFlags().BoolVar(
		&outlineOpts.Version,
		"version",
		true,
		"show versions along with package names",
	)
	outlineCmd.PersistentFlags().BoolVar(
		&outlineOpts.Purls,
		"purl",
		false,
		"show package urls instead of name@version",
	)

	parent.AddCommand(outlineCmd)
}
