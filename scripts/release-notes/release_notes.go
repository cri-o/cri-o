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

	"github.com/cri-o/cri-o/internal/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/release/pkg/command"
	"k8s.io/release/pkg/git"
	"k8s.io/release/pkg/util"
)

const branch = "gh-pages"

var outputPath string

func main() {
	// Parse CLI flags
	flag.StringVar(&outputPath,
		"output-path", "", "the output path for the release notes",
	)
	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	logrus.SetLevel(logrus.DebugLevel)
	command.SetGlobalVerbose(true)

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
	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		return errors.Wrap(err, "create output path")
	}

	// Get latest release version
	startTag := util.AddTagPrefix(decVersion(version.Version))
	logrus.Infof("Using start tag %s", startTag)

	endTag := util.AddTagPrefix(version.Version)
	logrus.Infof("Using end tag %s", endTag)

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

	targetBranch := git.DefaultBranch
	currentBranch, err := repo.CurrentBranch()
	if err != nil {
		return errors.Wrap(err, "get current branch")
	}
	logrus.Infof("Found current branch %s", currentBranch)
	if git.IsReleaseBranch(currentBranch) && currentBranch != git.DefaultBranch {
		targetBranch = currentBranch
	}
	logrus.Infof("Using target branch %s", targetBranch)

	templateFile, err := ioutil.TempFile("", "")
	if err != nil {
		return errors.Wrap(err, "writing template file")
	}
	defer func() { err = os.RemoveAll(templateFile.Name()) }()

	// Check if we're on a tag and adapt variables if necessary
	bundleVersion := head[:9]
	shortHead := head[:7]
	endRev := head
	if output, err := command.New(
		"git", "describe", "--exact-match",
	).RunSilentSuccessOutput(); err == nil {
		foundTag := output.OutputTrimNL()
		bundleVersion = foundTag
		shortHead = foundTag
		endRev = foundTag
	}

	if _, err := templateFile.WriteString(fmt.Sprintf(`# CRI-O %s

The release notes have been generated for the commit range
[%s...%s](https://github.com/cri-o/cri-o/compare/%s...%s) on %s.

## Downloads

Download the static release bundle via our Google Cloud Bucket:
[crio-%s.tar.gz][0]

[0]: https://storage.googleapis.com/k8s-conform-cri-o/artifacts/crio-%s.tar.gz

## Changelog since %s

{{with .NotesWithActionRequired -}}
### Urgent Upgrade Notes

{{range .}} {{println "-" .}} {{end}}
{{end}}

{{- if .Notes -}}
### Changes by Kind
{{ range .Notes}}
#### {{.Kind | prettyKind}}
{{range $note := .NoteEntries }}{{println " -" $note}}{{end}}
{{- end -}}
{{- end -}}
`,
		endTag,
		startTag, shortHead,
		startTag, endRev,
		time.Now().Format(time.RFC1123),
		bundleVersion, bundleVersion,
		startTag,
	)); err != nil {
		return errors.Wrap(err, "writing tmplate to file")
	}

	logrus.Infof("Generating release notes")
	outputFile := endTag + ".md"
	outputFilePath := filepath.Join(outputPath, outputFile)
	os.RemoveAll(outputFilePath)
	if err := command.Execute(
		"./build/bin/release-notes",
		"--github-org=cri-o",
		"--github-repo=cri-o",
		"--branch="+targetBranch,
		"--repo-path=/tmp/cri-o-repo",
		"--required-author=",
		"--start-rev="+startTag,
		"--end-sha="+head,
		"--output="+outputFilePath,
		"--toc",
		"--go-template=go-template:"+templateFile.Name(),
	); err != nil {
		return errors.Wrap(err, "generate release notes")
	}

	// Update gh-pages branch if not a pull request and running in CircleCI
	if util.IsEnvSet("CIRCLECI") && !util.IsEnvSet("CIRCLE_PULL_REQUEST") {
		content, err := ioutil.ReadFile(outputFilePath)
		if err != nil {
			return errors.Wrap(err, "open generated release notes")
		}

		logrus.Infof("Checking out branch %s", branch)
		if err := repo.Checkout(branch); err != nil {
			return errors.Wrapf(err, "checkout %s branch", branch)
		}
		defer func() { err = repo.Checkout(currentBranch) }()

		// Write the target file
		if err := ioutil.WriteFile(outputFile, content, 0o644); err != nil {
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

func decVersion(tag string) string {
	sv, err := util.TagStringToSemver(strings.TrimSpace(tag))
	if err != nil {
		panic(err)
	}

	// clear any RC
	sv.Pre = nil

	if sv.Patch > 0 { // nolint: gocritic
		sv.Patch-- // 1.17.2 -> 1.17.1
	} else if sv.Minor > 0 {
		sv.Minor-- // 1.18.0 -> 1.17.0
	} else if sv.Major > 0 {
		sv.Major-- // 1.19.0 -> 2.0.0 (should never happen)
	} else {
		panic(fmt.Sprintf("unable to decrement version %v", sv))
	}

	return sv.String()
}
