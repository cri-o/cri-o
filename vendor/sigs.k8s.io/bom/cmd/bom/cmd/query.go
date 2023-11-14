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
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"sigs.k8s.io/bom/pkg/query"
	"sigs.k8s.io/bom/pkg/spdx"
)

func AddQuery(parent *cobra.Command) {
	queryCmd := &cobra.Command{
		PersistentPreRunE: initLogging,
		Short:             "bom document query → Search for information in an SBOM",
		Long: `bom document query → Search for information in an SBOM

The query subcommand creates a way to extract information
from an SBOM. It exposes a simple search language to filter
elements in the sbom that match a certain criteria.

The query interface allows the number of filters to grow
over time. The following filters are available:

  depth:N       The depth filter will match elements
                reachable at N levels from the document root.

  name:pattern  Matches all elements in the document that
                contain <pattern> in their name.

Example:

  # Match all second level elements with log4j in their name:
  bom document query sbom.spdx "depth:2 name:log4j"

`,
		Use:           "query SPDX_FILE|URL \"query expression\" ",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				cmd.Help() //nolint:errcheck
				return errors.New("you should only specify one file")
			}

			q := query.New()
			if err := q.Open(args[0]); err != nil {
				return fmt.Errorf("opening document %s: %w", args[0], err)
			}
			fp, err := q.Query(strings.Join(args[1:], " "))
			if err != nil {
				return fmt.Errorf("querying document: %w", err)
			}

			if fp.Error != nil {
				return fmt.Errorf("filter query returned an error: %w", fp.Error)
			}

			fmt.Println(spdx.Banner())
			if len(fp.Objects) == 0 {
				logrus.Warning("No objects in the SBOM match the query")
				return nil
			}
			for _, o := range fp.Objects {
				s := fmt.Sprintf("[NO NAME; ID=%s]", o.SPDXID())
				switch no := o.(type) {
				case *spdx.File:
					s = no.FileName
				case *spdx.Package:
					s = no.Name
				}
				fmt.Printf(" %s\n", s)
			}
			return nil
		},
	}

	parent.AddCommand(queryCmd)
}
