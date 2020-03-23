package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/release/pkg/command"
	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/util"
)

const branch = "gh-pages"

var (
	outputPath string
	tag        string
)

func main() {
	// Parse CLI flags
	flag.StringVar(&tag, "tag", "", "the release tag of the notes")
	flag.StringVar(&outputPath,
		"output-path", "", "the output path for the release notes",
	)
	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	if err := run(); err != nil {
		logrus.Fatalf("unable to %v", err)
	}
}

func run() error {
	// Precheck environemt
	if !util.IsEnvSet("GITHUB_TOKEN") {
		return errors.Errorf("GITHUB_TOKEN environemt variable is not set")
	}

	logrus.Infof("Ensuring output path %s", outputPath)
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return errors.Wrap(err, "create output path")
	}

	// Get latest release version
	res, err := command.New(
		"go", "run", "./scripts/latest-version", "--no-bump-version",
	).RunSilentSuccessOutput()
	if err != nil {
		return errors.Wrap(err, "retrieve start tag")
	}
	startTag := util.AddTagPrefix(res.OutputTrimNL())
	endTag := util.AddTagPrefix(tag)
	logrus.Infof("Using start tag %s", startTag)

	// Generate the notes
	repo, err := git.OpenRepo(".")
	if err != nil {
		return errors.Wrap(err, "open local repo")
	}

	head, err := repo.Head()
	if err != nil {
		return errors.Wrap(err, "get repository HEAD")
	}
	logrus.Infof("Using HEAD commit %s", head)

	logrus.Infof("Generating release notes")
	outputFile := endTag + ".md"
	outputFilePath := filepath.Join(outputPath, outputFile)
	os.RemoveAll(outputFilePath)
	if err := command.Execute(
		"./build/bin/release-notes",
		"--github-org=cri-o",
		"--github-repo=cri-o",
		"--repo-path=/tmp/cri-o-repo",
		"--required-author=",
		"--start-rev="+startTag,
		"--end-sha="+head,
		"--output="+outputFilePath,
	); err != nil {
		return errors.Wrap(err, "generate release notes")
	}

	// Postprocess the notes
	content, err := ioutil.ReadFile(outputFilePath)
	if err != nil {
		return errors.Wrap(err, "open generated release notes")
	}

	finalContent := fmt.Sprintf(`# CRI-O %s

The release notes have been generated for the commit range
[%s...%s](https://github.com/cri-o/cri-o/compare/%s...%s) on %s.

## Downloads

Download the static release bundle via our Google Cloud Bucket:
[crio-%s.tar.gz][0]

[0]: https://storage.googleapis.com/k8s-conform-cri-o/artifacts/crio-%s.tar.gz

`+string(content),
		endTag,
		startTag, head[:7],
		startTag, head,
		time.Now().Format(time.RFC1123),
		head[:9], head[:9],
	)

	if err := ioutil.WriteFile(
		outputFilePath, []byte(finalContent), 0o644,
	); err != nil {
		return errors.Wrap(err, "write content to file")
	}

	// Update gh-pages branch if not a pull request and running in CircleCI
	if util.IsEnvSet("CIRCLECI") && !util.IsEnvSet("CIRCLE_PULL_REQUEST") {
		currentBranch, err := repo.CurrentBranch()
		if err != nil {
			return errors.Wrapf(err, "get current branch")
		}

		logrus.Infof("Checking out branch %s", branch)
		if err := repo.Checkout(branch); err != nil {
			return errors.Wrapf(err, "checkout %s branch", branch)
		}
		defer func() { err = repo.Checkout(currentBranch) }()

		// Write the target file
		if err := ioutil.WriteFile(
			outputFile, []byte(finalContent), 0o644,
		); err != nil {
			return errors.Wrap(err, "write content to file")
		}

		if err := repo.Add(outputFile); err != nil {
			return errors.Wrap(err, "add file to repo")
		}

		// Update the README
		readmeFile := "README.md"
		logrus.Infof("Updating %s", readmeFile)
		readmeSlice, err := readLines(readmeFile)
		if err != nil {
			return errors.Wrapf(err, "open %s file", readmeFile)
		}
		link := fmt.Sprintf("- [%s](%s)", endTag, outputFile)

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
			return errors.Wrap(err, "write content to file")
		}
		if err := repo.Add(readmeFile); err != nil {
			return errors.Wrap(err, "add file to repo")
		}

		// Publish the changes
		if err := repo.Commit("Update release notes"); err != nil {
			return errors.Wrap(err, "commit")
		}

		if err := repo.Push(branch); err != nil {
			return errors.Wrap(err, "push changes")
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
