package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/release/pkg/command"
	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/github"
	"k8s.io/release/pkg/util"
)

const branch = "gh-pages"

var (
	commit     bool
	outputPath string
	tag        string
)

func main() {
	// Parse CLI flags
	flag.BoolVar(&commit, "commit", false, fmt.Sprintf(
		"commit and push the changes into the local %s branch", branch))
	flag.StringVar(&tag, "tag", "", "the release tag of the notes")
	flag.StringVar(&outputPath,
		"output-path", "", "the output path for the release notes",
	)
	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	if err := run(); err != nil {
		logrus.Fatal(err)
	}
}

func run() error {
	// Precheck environemt
	if !util.IsEnvSet(github.TokenEnvKey) {
		return errors.Errorf(
			"%s environemt variable is not set", github.TokenEnvKey,
		)
	}

	logrus.Infof("Ensuring output path %s", outputPath)
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return errors.Wrap(err, "unable to create output path")
	}

	// Get latest release version
	res, err := command.New(
		"go", "run", "./scripts/latest-version", "--no-bump-version",
	).RunSilentSuccessOutput()
	if err != nil {
		return errors.Wrap(err, "unable to retrieve start tag")
	}
	startTag := util.AddTagPrefix(res.OutputTrimNL())
	logrus.Infof("Using start tag %s", startTag)

	// Generate the notes
	repo, err := git.OpenRepo(".")
	if err != nil {
		return errors.Wrap(err, "unable to open local repo")
	}

	head, err := repo.Head()
	if err != nil {
		return errors.Wrap(err, "unable to get repository HEAD")
	}

	logrus.Infof("Generating release notes")
	outputFile := tag + ".md"
	outputFilePath := filepath.Join(outputPath, outputFile)
	if err := command.Execute(
		"./build/bin/release-notes",
		"--github-org=cri-o",
		"--github-repo=cri-o",
		"--required-author=",
		"--start-rev="+startTag,
		"--end-sha="+head,
		"--output="+outputFilePath,
	); err != nil {
		return errors.Wrap(err, "unable to generate release notes")
	}

	// Postprocess the notes
	content, err := ioutil.ReadFile(outputFilePath)
	if err != nil {
		return errors.Wrap(err, "unable to open generated release notes")
	}

	finalContent := fmt.Sprintf("# CRI-O %s\n\n"+
		"The release notes have been generated based on commit\n"+
		"[%s](https://github.com/cri-o/cri-o/commit/%s).\n\n%s",
		util.AddTagPrefix(tag), head[:7], head, content,
	)

	if err := ioutil.WriteFile(
		outputFilePath, []byte(finalContent), 0o644,
	); err != nil {
		return errors.Wrap(err, "unable to write content to file")
	}

	// Update gh-pages branch
	if commit {
		currentBranch, err := repo.CurrentBranch()
		if err != nil {
			return errors.Wrapf(err, "unable to get current branch")
		}

		if err := repo.Checkout(branch); err != nil {
			return errors.Wrapf(err, "unable to checkout %s branch", branch)
		}
		defer func() { err = repo.Checkout(currentBranch) }()

		// Write the target file
		if err := ioutil.WriteFile(
			outputFile, []byte(finalContent), 0o644,
		); err != nil {
			return errors.Wrap(err, "unable to write content to file")
		}

		if err := repo.Add(outputFile); err != nil {
			return errors.Wrap(err, "unable to add file to repo")
		}

		// Update the README
		readmeFile := "README.md"
		readmeSlice, err := readLines(readmeFile)
		if err != nil {
			return errors.Wrapf(err, "unable to open %s file", readmeFile)
		}
		link := fmt.Sprintf("- [%s](%s)", util.AddTagPrefix(tag), outputFile)

		// Item not in list
		alreadyExistingIndex := indexOfPrefix(link, readmeSlice)
		if alreadyExistingIndex < 0 {
			firstListEntry := indexOfPrefix("- ", readmeSlice)

			if firstListEntry < 0 {
				// No list available, just append
				readmeSlice = append(readmeSlice, link)
			} else {
				// Insert into slice
				readmeSlice = append(
					readmeSlice[:firstListEntry],
					append([]string{link}, readmeSlice[firstListEntry:]...)...,
				)
			}
		} else {
			readmeSlice[alreadyExistingIndex] = link
		}
		if err := ioutil.WriteFile(
			readmeFile, []byte(strings.Join(readmeSlice, "\n")), 0o644,
		); err != nil {
			return errors.Wrap(err, "unable to write content to file")
		}
		if err := repo.Add(readmeFile); err != nil {
			return errors.Wrap(err, "unable to add file to repo")
		}

		// Publish the changes
		if err := repo.Commit("Update release notes"); err != nil {
			return errors.Wrap(err, "unable to commit")
		}

		if err := repo.Push(git.Remotify(branch)); err != nil {
			return errors.Wrap(err, "unable to push changes")
		}
	}

	return nil
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func indexOfPrefix(prefix string, slice []string) int {
	for k, v := range slice {
		if strings.HasPrefix(v, prefix) {
			return k
		}
	}
	return -1
}
