/*
Copyright 2019 The Kubernetes Authors.

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

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/log"
	"k8s.io/release/pkg/notes"
	"k8s.io/release/pkg/notes/document"
	"k8s.io/release/pkg/notes/options"
	"k8s.io/release/pkg/release"
	"k8s.io/release/pkg/util"
	"sigs.k8s.io/mdtoc/pkg/mdtoc"
)

type releaseNotesOptions struct {
	outputFile      string
	tableOfContents bool
	dependencies    bool
}

var (
	releaseNotesOpts = &releaseNotesOptions{}
	opts             = options.New()
	cmd              = &cobra.Command{
		Short:         "release-notes - The Kubernetes Release Notes Generator",
		Use:           "release-notes",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          run,
		PreRunE: func(*cobra.Command, []string) error {
			return opts.ValidateAndFinish()
		},
	}
)

func init() {
	// githubOrg contains name of github organization that holds the repo to scrape.
	cmd.PersistentFlags().StringVar(
		&opts.GithubOrg,
		"org",
		util.EnvDefault("ORG", notes.DefaultOrg),
		"Name of github organization",
	)

	// githubRepo contains name of github repository to scrape.
	cmd.PersistentFlags().StringVar(
		&opts.GithubRepo,
		"repo",
		util.EnvDefault("REPO", notes.DefaultRepo),
		"Name of github repository",
	)

	// output contains the path on the filesystem to where the resultant
	// release notes should be printed.
	cmd.PersistentFlags().StringVar(
		&releaseNotesOpts.outputFile,
		"output",
		util.EnvDefault("OUTPUT", ""),
		"The path to the where the release notes will be printed",
	)

	// branch is which branch to scrape.
	cmd.PersistentFlags().StringVar(
		&opts.Branch,
		"branch",
		util.EnvDefault("BRANCH", git.DefaultBranch),
		fmt.Sprintf("Select which branch to scrape. Defaults to `%s`", git.DefaultBranch),
	)

	// startSHA contains the commit SHA where the release note generation
	// begins.
	cmd.PersistentFlags().StringVar(
		&opts.StartSHA,
		"start-sha",
		util.EnvDefault("START_SHA", ""),
		"The commit hash to start at",
	)

	// endSHA contains the commit SHA where the release note generation ends.
	cmd.PersistentFlags().StringVar(
		&opts.EndSHA,
		"end-sha",
		util.EnvDefault("END_SHA", ""),
		"The commit hash to end at",
	)

	// startRev contains any valid git object where the release note generation
	// begins. Can be used as alternative to start-sha.
	cmd.PersistentFlags().StringVar(
		&opts.StartRev,
		"start-rev",
		util.EnvDefault("START_REV", ""),
		"The git revision to start at. Can be used as alternative to start-sha.",
	)

	// endRev contains any valid git object where the release note generation
	// ends. Can be used as alternative to start-sha.
	cmd.PersistentFlags().StringVar(
		&opts.EndRev,
		"end-rev",
		util.EnvDefault("END_REV", ""),
		"The git revision to end at. Can be used as alternative to end-sha.",
	)

	// repoPath contains the path to a local Kubernetes repository to avoid the
	// delay during git clone
	cmd.PersistentFlags().StringVar(
		&opts.RepoPath,
		"repo-path",
		util.EnvDefault("REPO_PATH", filepath.Join(os.TempDir(), "k8s-repo")),
		"Path to a local Kubernetes repository, used only for tag discovery.",
	)

	// format is the output format to produce the notes in.
	cmd.PersistentFlags().StringVar(
		&opts.Format,
		"format",
		util.EnvDefault("FORMAT", options.FormatMarkdown),
		fmt.Sprintf("The format for notes output (options: %s)",
			strings.Join([]string{
				options.FormatJSON,
				options.FormatMarkdown,
			}, ", "),
		),
	)

	// go-template is the go template to be used when the format is markdown
	cmd.PersistentFlags().StringVar(
		&opts.GoTemplate,
		"go-template",
		util.EnvDefault("GO_TEMPLATE", options.GoTemplateDefault),
		fmt.Sprintf("The go template to be used if --format=markdown (options: %s)",
			strings.Join([]string{
				options.GoTemplateDefault,
				options.GoTemplateInline + "<template>",
				options.GoTemplatePrefix + "<file.template>",
			}, ", "),
		),
	)

	cmd.PersistentFlags().StringVar(
		&opts.RequiredAuthor,
		"required-author",
		util.EnvDefault("REQUIRED_AUTHOR", "k8s-ci-robot"),
		"Only commits from this GitHub user are considered. Set to empty string to include all users",
	)

	cmd.PersistentFlags().BoolVar(
		&opts.Debug,
		"debug",
		util.IsEnvSet("DEBUG"),
		"Enable debug logging",
	)

	cmd.PersistentFlags().StringVar(
		&opts.DiscoverMode,
		"discover",
		util.EnvDefault("DISCOVER", options.RevisionDiscoveryModeNONE),
		fmt.Sprintf("The revision discovery mode for automatic revision retrieval (options: %s)",
			strings.Join([]string{
				options.RevisionDiscoveryModeNONE,
				options.RevisionDiscoveryModeMergeBaseToLatest,
				options.RevisionDiscoveryModePatchToPatch,
				options.RevisionDiscoveryModeMinorToMinor,
			}, ", "),
		),
	)

	cmd.PersistentFlags().StringVar(
		&opts.ReleaseBucket,
		"release-bucket",
		util.EnvDefault("RELEASE_BUCKET", release.ProductionBucket),
		"Specify gs bucket to point to in generated notes",
	)

	cmd.PersistentFlags().StringVar(
		&opts.ReleaseTars,
		"release-tars",
		util.EnvDefault("RELEASE_TARS", ""),
		"Directory of tars to sha512 sum for display",
	)

	cmd.PersistentFlags().BoolVar(
		&releaseNotesOpts.tableOfContents,
		"toc",
		util.IsEnvSet("TOC"),
		"Enable the rendering of the table of contents",
	)

	cmd.PersistentFlags().StringVar(
		&opts.RecordDir,
		"record",
		util.EnvDefault("RECORD", ""),
		"Record the API into a directory",
	)

	cmd.PersistentFlags().StringVar(
		&opts.ReplayDir,
		"replay",
		util.EnvDefault("REPLAY", ""),
		"Replay a previously recorded API from a directory",
	)

	cmd.PersistentFlags().BoolVar(
		&releaseNotesOpts.dependencies,
		"dependencies",
		true,
		"Add dependency report",
	)

	cmd.PersistentFlags().StringSliceVarP(
		&opts.MapProviderStrings,
		"maps-from",
		"m",
		[]string{},
		"specify a location to recursively look for release notes *.y[a]ml file mappings",
	)
}

func WriteReleaseNotes(releaseNotes *notes.ReleaseNotes) (err error) {
	logrus.Infof(
		"Got %d release notes, performing rendering",
		len(releaseNotes.History()),
	)

	var (
		// Open a handle to the file which will contain the release notes output
		output        *os.File
		existingNotes notes.ReleaseNotesByPR
	)

	if releaseNotesOpts.outputFile != "" {
		output, err = os.OpenFile(releaseNotesOpts.outputFile, os.O_RDWR|os.O_CREATE, os.FileMode(0o644))
		if err != nil {
			return errors.Wrapf(err, "opening the supplied output file")
		}
	} else {
		output, err = ioutil.TempFile("", "release-notes-")
		if err != nil {
			return errors.Wrapf(err, "creating a temporary file to write the release notes to")
		}
	}

	// Contextualized release notes can be printed in a variety of formats
	if opts.Format == options.FormatJSON {
		byteValue, err := ioutil.ReadAll(output)
		if err != nil {
			return err
		}

		if len(byteValue) > 0 {
			if err := json.Unmarshal(byteValue, &existingNotes); err != nil {
				return errors.Wrapf(err, "unmarshalling existing notes")
			}
		}

		if len(existingNotes) > 0 {
			if err := output.Truncate(0); err != nil {
				return err
			}
			if _, err := output.Seek(0, 0); err != nil {
				return err
			}

			for i := 0; i < len(existingNotes); i++ {
				pr := existingNotes[i].PrNumber
				if releaseNotes.Get(pr) == nil {
					releaseNotes.Set(pr, existingNotes[i])
				}
			}
		}

		enc := json.NewEncoder(output)
		enc.SetIndent("", "  ")
		if err := enc.Encode(releaseNotes.ByPR()); err != nil {
			return errors.Wrapf(err, "encoding JSON output")
		}
	} else {
		doc, err := document.New(releaseNotes, opts.StartRev, opts.EndRev)
		if err != nil {
			return errors.Wrapf(err, "creating release note document")
		}

		markdown, err := doc.RenderMarkdownTemplate(opts.ReleaseBucket, opts.ReleaseTars, opts.GoTemplate)
		if err != nil {
			return errors.Wrapf(err, "rendering release note document with template")
		}

		const nl = "\n"
		if releaseNotesOpts.dependencies {
			if opts.StartSHA == opts.EndSHA {
				logrus.Info("Skipping dependency report because start and end SHA are the same")
			} else {
				url := git.GetRepoURL(opts.GithubOrg, opts.GithubRepo, false)
				deps, err := notes.NewDependencies().ChangesForURL(
					url, opts.StartSHA, opts.EndSHA,
				)
				if err != nil {
					return errors.Wrap(err, "generating dependency report")
				}
				markdown += strings.Repeat(nl, 2) + deps
			}
		}

		if releaseNotesOpts.tableOfContents {
			toc, err := mdtoc.GenerateTOC([]byte(markdown))
			if err != nil {
				return errors.Wrap(err, "generating table of contents")
			}
			markdown = toc + nl + markdown
		}

		if _, err := output.WriteString(markdown); err != nil {
			return errors.Wrap(err, "writing output file")
		}
	}

	logrus.Infof("Release notes written to file: %s", output.Name())
	return nil
}

func run(*cobra.Command, []string) error {
	releaseNotes, err := notes.GatherReleaseNotes(opts)
	if err != nil {
		return errors.Wrapf(err, "gathering release notes")
	}

	return WriteReleaseNotes(releaseNotes)
}

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	logrus.AddHook(log.NewFilenameHook())
	if err := cmd.Execute(); err != nil {
		logrus.Fatal(err)
	}
}
