package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/release-sdk/git"
	"sigs.k8s.io/release-utils/command"
)

const (
	branch   = "gh-pages"
	file     = "dependencies.md"
	tokenKey = "GITHUB_TOKEN"
)

var outputPath string

func main() {
	// Parse CLI flags
	flag.StringVar(&outputPath,
		"output-path", "", "the output path for the release notes",
	)
	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

	if err := run(); err != nil {
		logrus.Fatalf("Unable to %v", err)
	}
}

func run() error {
	// Ensure output path
	logrus.Infof("Ensuring output path %s", outputPath)

	if err := os.MkdirAll(outputPath, 0o755); err != nil {
		return fmt.Errorf("create output path: %w", err)
	}

	// Generate the report
	logrus.Infof("Getting go modules")

	if err := os.Setenv("GOSUMDB", "off"); err != nil {
		return fmt.Errorf("disabling GOSUMDB: %w", err)
	}

	modules, err := command.New(
		"go", "list", "--mod=mod", "-u", "-m", "--json", "all",
	).RunSilentSuccessOutput()
	if err != nil {
		return fmt.Errorf("listing go modules: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "modules-")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	if _, err := tmpFile.WriteString(modules.OutputTrimNL()); err != nil {
		return fmt.Errorf("writing to temp file: %w", err)
	}

	logrus.Infof("Retrieving outdated dependencies")

	outdated, err := command.New("cat", tmpFile.Name()).
		Pipe("./build/bin/go-mod-outdated", "--direct", "--update", "--style=markdown").
		RunSuccessOutput()
	if err != nil {
		return fmt.Errorf("retrieving outdated dependencies: %w", err)
	}

	logrus.Infof("Retrieving all dependencies")

	all, err := command.New("cat", tmpFile.Name()).
		Pipe("./build/bin/go-mod-outdated", "--style=markdown").
		RunSuccessOutput()
	if err != nil {
		return fmt.Errorf("retrieving all dependencies: %w", err)
	}

	// Write the output
	outputFile := filepath.Join(outputPath, file)
	os.RemoveAll(outputFile)

	repo, err := git.OpenRepo(".")
	if err != nil {
		return fmt.Errorf("open local repo: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return fmt.Errorf("get repository HEAD: %w", err)
	}

	content := fmt.Sprintf(`# CRI-O Dependency Report

_Generated on %s for commit [%s][0]._

[0]: https://github.com/cri-o/cri-o/commit/%s

## Outdated Dependencies

%s

## All Dependencies

%s
`,
		time.Now().Format(time.RFC1123),
		head[:7], head,
		outdated.OutputTrimNL(),
		all.OutputTrimNL(),
	)
	if err := os.WriteFile(outputFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	token, tokenSet := os.LookupEnv(tokenKey)
	if !tokenSet || token == "" {
		logrus.Infof("%s environment variable is not set", tokenKey)
		os.Exit(0)
	}

	currentBranch, err := repo.CurrentBranch()
	if err != nil {
		return fmt.Errorf("get current branch: %w", err)
	}

	logrus.Infof("Checking out branch %s", branch)

	if err := repo.Checkout(branch); err != nil {
		return fmt.Errorf("checkout %s branch: %w", branch, err)
	}

	defer func() { err = repo.Checkout(currentBranch) }()

	// Write the target file
	logrus.Infof("Writing dependency report to %s", file)

	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write content to file: %w", err)
	}

	if err := repo.Add(file); err != nil {
		return fmt.Errorf("add file to repo: %w", err)
	}

	// Publish the changes
	logrus.Info("Committing changes")

	if err := repo.Commit("Update dependency report"); err != nil {
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
