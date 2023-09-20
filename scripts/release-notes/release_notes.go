package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/google/go-github/v50/github"
	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-sdk/git"
	"sigs.k8s.io/release-utils/command"
	"sigs.k8s.io/release-utils/util"
)

const (
	branch           = "gh-pages"
	currentBranchKey = "CURRENT_BRANCH"
	tokenKey         = "GITHUB_TOKEN"
	defaultBranch    = "main"
)

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
		logrus.Fatalf("Unable to %v", err)
	}
}

func run() error {
	// Precheck environment.
	token, tokenSet := os.LookupEnv(tokenKey)
	if !tokenSet || token == "" {
		logrus.Infof("%s environment variable is not set", tokenKey)
		os.Exit(0)
	}

	logrus.Infof("Ensuring output path %s", outputPath)
	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		return fmt.Errorf("create output path: %w", err)
	}

	// Get latest release version
	releaseVersion, err := getLatestReleaseVersion(token)
	if err != nil {
		return fmt.Errorf("error while listing the github tags: %w", err)
	}
	startTag := util.AddTagPrefix(decVersion(releaseVersion))
	logrus.Infof("Using start tag %s", startTag)

	endTag := util.AddTagPrefix(releaseVersion)
	logrus.Infof("Using end tag %s", endTag)

	// Generate the notes
	repo, err := git.OpenRepo(".")
	if err != nil {
		return fmt.Errorf("open local repo: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("get repository HEAD: %w", err)
	}
	logrus.Infof("Using HEAD commit %s", head)

	currentBranch, currentBranchSet := os.LookupEnv(currentBranchKey)
	if !currentBranchSet || currentBranch == "" {
		logrus.Infof(
			"%s environment variable is not set, using default branch `%s`",
			currentBranchKey, defaultBranch,
		)
		currentBranch = defaultBranch
	}
	logrus.Infof("Using branch: %s", currentBranch)

	templateFile, err := os.CreateTemp("", "")
	if err != nil {
		return fmt.Errorf("writing template file: %w", err)
	}
	defer func() { err = os.RemoveAll(templateFile.Name()) }()

	// Check if we're on a tag and adapt variables if necessary
	bundleVersion := head
	shortHead := head[:7]
	endRev := head
	if output, err := command.New(
		"git", "describe", "--tags", "--exact-match",
	).RunSilentSuccessOutput(); err == nil {
		foundTag := output.OutputTrimNL()
		logrus.Infof("Using tag via `git describe`: %s", foundTag)
		bundleVersion = foundTag
		shortHead = foundTag
		endRev = foundTag
	} else {
		logrus.Infof("Not using git tag because `git describe` failed: %v", err)
	}

	if _, err := fmt.Fprintf(templateFile, `# CRI-O %s

The release notes have been generated for the commit range
[%s...%s](https://github.com/cri-o/cri-o/compare/%s...%s) on %s.

## Downloads

Download one of our static release bundles via our Google Cloud Bucket:

- [cri-o.amd64.%s.tar.gz](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.%s.tar.gz)
  - [cri-o.amd64.%s.tar.gz.sha256sum](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.%s.tar.gz.sha256sum)
  - [cri-o.amd64.%s.tar.gz.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.%s.tar.gz.sig)
  - [cri-o.amd64.%s.tar.gz.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.%s.tar.gz.cert)
  - [cri-o.amd64.%s.tar.gz.spdx](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.%s.tar.gz.spdx)
  - [cri-o.amd64.%s.tar.gz.spdx.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.%s.tar.gz.spdx.sig)
  - [cri-o.amd64.%s.tar.gz.spdx.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.%s.tar.gz.spdx.cert)
- [cri-o.arm64.%s.tar.gz](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.%s.tar.gz)
  - [cri-o.arm64.%s.tar.gz.sha256sum](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.%s.tar.gz.sha256sum)
  - [cri-o.arm64.%s.tar.gz.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.%s.tar.gz.sig)
  - [cri-o.arm64.%s.tar.gz.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.%s.tar.gz.cert)
  - [cri-o.arm64.%s.tar.gz.spdx](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.%s.tar.gz.spdx)
  - [cri-o.arm64.%s.tar.gz.spdx.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.%s.tar.gz.spdx.sig)
  - [cri-o.arm64.%s.tar.gz.spdx.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.arm64.%s.tar.gz.spdx.cert)
- [cri-o.ppc64le.%s.tar.gz](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.%s.tar.gz)
  - [cri-o.ppc64le.%s.tar.gz.sha256sum](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.%s.tar.gz.sha256sum)
  - [cri-o.ppc64le.%s.tar.gz.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.%s.tar.gz.sig)
  - [cri-o.ppc64le.%s.tar.gz.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.%s.tar.gz.cert)
  - [cri-o.ppc64le.%s.tar.gz.spdx](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.%s.tar.gz.spdx)
  - [cri-o.ppc64le.%s.tar.gz.spdx.sig](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.%s.tar.gz.spdx.sig)
  - [cri-o.ppc64le.%s.tar.gz.spdx.cert](https://storage.googleapis.com/cri-o/artifacts/cri-o.ppc64le.%s.tar.gz.spdx.cert)

To verify the artifact signatures via [cosign](https://github.com/sigstore/cosign), run:

`+"```"+`console
> export COSIGN_EXPERIMENTAL=1
> cosign verify-blob cri-o.amd64.%s.tar.gz \
    --certificate-identity https://github.com/cri-o/cri-o/.github/workflows/test.yml@refs/tags/%s \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-github-workflow-repository cri-o/cri-o \
    --certificate-github-workflow-ref refs/tags/%s \
    --signature cri-o.amd64.%s.tar.gz.sig \
    --certificate cri-o.amd64.%s.tar.gz.cert
`+"```"+`

To verify the bill of materials (SBOM) in [SPDX](https://spdx.org) format using the [bom](https://sigs.k8s.io/bom) tool, run:

`+"```"+`console
> tar xfz cri-o.amd64.%s.tar.gz
> bom validate -e cri-o.amd64.%s.tar.gz.spdx -d cri-o
`+"```"+`

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
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		bundleVersion, bundleVersion, bundleVersion,
		bundleVersion, bundleVersion,
		startTag,
	); err != nil {
		return fmt.Errorf("writing tmplate to file: %w", err)
	}

	logrus.Infof("Generating release notes")
	outputFile := endTag + ".md"
	outputFilePath := filepath.Join(outputPath, outputFile)
	os.RemoveAll(outputFilePath)
	if err := command.Execute(
		"./build/bin/release-notes",
		"--org=cri-o",
		"--repo=cri-o",
		"--branch="+currentBranch,
		"--repo-path=/tmp/cri-o-repo",
		"--required-author=",
		"--start-rev="+startTag,
		"--end-sha="+head,
		"--output="+outputFilePath,
		"--toc",
		"--go-template=go-template:"+templateFile.Name(),
	); err != nil {
		return fmt.Errorf("generate release notes: %w", err)
	}

	content, err := os.ReadFile(outputFilePath)
	if err != nil {
		return fmt.Errorf("open generated release notes: %w", err)
	}

	logrus.Infof("Checking out branch %s", branch)
	if err := repo.Checkout(branch); err != nil {
		return fmt.Errorf("checkout %s branch: %w", branch, err)
	}
	defer func() { err = repo.Checkout(currentBranch) }()

	// Write the target file
	if err := os.WriteFile(outputFile, content, 0o644); err != nil {
		return fmt.Errorf("write content to file: %w", err)
	}

	if err := repo.Add(outputFile); err != nil {
		return fmt.Errorf("add file to repo: %w", err)
	}

	// Update the README
	readmeFile := "README.md"
	logrus.Infof("Updating %s", readmeFile)
	readmeSlice, err := readLines(readmeFile)
	if err != nil {
		return fmt.Errorf("open %s file: %w", readmeFile, err)
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
	if err := os.WriteFile(
		readmeFile, []byte(strings.Join(readmeSlice, "\n")), 0o644,
	); err != nil {
		return fmt.Errorf("write content to file: %w", err)
	}
	if err := repo.Add(readmeFile); err != nil {
		return fmt.Errorf("add file to repo: %w", err)
	}

	// Publish the changes
	if err := repo.Commit("Update release notes"); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Other jobs could run in parallel, try rebase multiple times before
	// pushing
	const maxRetries = 10
	for i := 0; i <= maxRetries; i++ {
		if err := command.New("git", "pull", "--rebase").RunSuccess(); err != nil {
			return fmt.Errorf("pull and rebase from remote: %w", err)
		}

		err := repo.Push(branch)
		if err == nil {
			break
		}

		if i == maxRetries {
			return fmt.Errorf("max retries reached for pushing changes: %w", err)
		}

		logrus.Warnf("Failed to push changes, retrying (%d): %v", i, err)
		time.Sleep(3 * time.Second)
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

// getLatestReleaseVersion fetches the latest github release
// version from cri-o.
func getLatestReleaseVersion(token string) (string, error) {
	ctx := context.Background()
	client := github.NewTokenClient(ctx, token)
	tags, _, err := client.Repositories.ListTags(ctx, "cri-o", "cri-o", &github.ListOptions{PerPage: 10})
	if err != nil {
		return "", err
	}
	svers := []semver.Version{}
	for _, tag := range tags {
		v := semver.MustParse(strings.SplitAfter(tag.GetName(), "v")[1])
		svers = append(svers, v)
	}
	semver.Sort(svers)
	if len(svers) == 0 {
		return "", errors.New("no tags found")
	}
	return svers[len(svers)-1].String(), nil
}