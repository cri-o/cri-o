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

package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"sigs.k8s.io/zeitgeist/dependency"
)

func addUpgrade(topLevel *cobra.Command) {
	vo := rootOpts

	cmd := &cobra.Command{
		Use:           "upgrade",
		Short:         "Upgrade local dependencies based on upstream versions",
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE: func(*cobra.Command, []string) error {
			return vo.setAndValidate()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(vo)
		},
	}

	topLevel.AddCommand(cmd)
}

// runUpgrade is the function invoked by 'addUpgrade', responsible for
// upgrading dependencies.
func runUpgrade(opts *options) error {
	client := dependency.NewClient()

	// Check locally first: it's fast, and ensures we're working on clean files
	if err := client.LocalCheck(opts.configFile, opts.basePath); err != nil {
		return fmt.Errorf("checking local dependencies: %w", err)
	}

	updates, err := client.Upgrade(opts.configFile)
	if err != nil {
		return fmt.Errorf("upgrade dependencies: %w", err)
	}

	for _, update := range updates {
		fmt.Println(update)
	}

	return nil
}
