// This tools automatically finds the latest CRI-O release branch and merges
// the latest master branch into it. This happens only if there is no
// tag present on the release branch.
package main

import (
	"flag"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/release/pkg/command"
	kgit "k8s.io/release/pkg/git"
	"k8s.io/release/pkg/util"
)

const (
	remote              = "https://github.com/cri-o/cri-o"
	git                 = "git"
	grep                = "grep"
	tail                = "tail"
	releaseBranchPrefix = "release-"
	dryRunEnv           = "DRY_RUN"
)

var dryRun bool

func main() {
	flag.BoolVar(
		&dryRun,
		"dry-run",
		!util.IsEnvSet(dryRunEnv),
		"do not really push, just do a dry run",
	)
	flag.Parse()

	logrus.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})
	if err := run(); err != nil {
		logrus.Fatal(err)
	}
}

func run() error {
	if !command.Available(git, grep, tail) {
		return errors.Errorf(
			"please ensure that %s are available in $PATH",
			strings.Join([]string{git, grep, tail}, ", "),
		)
	}

	if dryRun {
		logrus.Warnf("Please note that this is only a dry-run and will not "+
			"result in any real git push to the remote location. "+
			"To enable a real git push, export the environment "+
			"variable %s=false",
			dryRunEnv,
		)
	}

	// Get the latest release branch
	lsRemoteHeads, err := command.
		New(git, "ls-remote", "--sort=v:refname", "--heads", remote).
		Pipe(grep, "-Eo", releaseBranchPrefix+".*").
		Pipe(tail, "-1").
		RunSilentSuccessOutput()
	if err != nil {
		return errors.Wrap(err, "unable to retrieve latest release branch")
	}
	latestReleaseBranch := lsRemoteHeads.OutputTrimNL()
	logrus.Infof("Latest release branch: %s", latestReleaseBranch)

	// Check if a release has been done on that branch
	tagPrefix := strings.TrimPrefix(latestReleaseBranch, releaseBranchPrefix)
	lsRemoteTags, err := command.
		New(git, "ls-remote", "--sort=v:refname", "--tags", remote).
		Pipe(grep, "v"+tagPrefix).
		RunSilentSuccessOutput()
	if err == nil {
		logrus.Warnf(
			"Found existing tag(s) on release branch: %v",
			strings.Join(strings.Fields(lsRemoteTags.OutputTrimNL()), ", "),
		)
		logrus.Infof("We’re all set, doing nothing")
		return nil
	}

	// Checkout the release branch
	repo, err := kgit.OpenRepo(".")
	if err != nil {
		return errors.Wrap(err, "unable to open this repository")
	}
	if dryRun {
		logrus.Info("Setting repository to only do a dry-run")
		repo.SetDry()
	}
	currentBranch, err := repo.CurrentBranch()
	if err != nil {
		return errors.Wrap(err, "unable to get current branch")
	}
	logrus.Infof("Checking out branch: %s", latestReleaseBranch)
	if err := repo.Checkout(latestReleaseBranch); err != nil {
		return errors.Wrapf(err,
			"unable to checkout release branch %s", latestReleaseBranch,
		)
	}
	defer func() {
		logrus.Infof("Checking out branch: %s", currentBranch)
		err = repo.Checkout(currentBranch)
	}()

	// Merge the latest master
	mergeTarget := kgit.Remotify(kgit.DefaultBranch)
	if err := repo.Merge(mergeTarget); err != nil {
		return errors.Wrapf(err,
			"unable to merge %s into release branch", mergeTarget,
		)
	}

	// Push the changes
	if err := repo.Push(latestReleaseBranch); err != nil {
		return errors.Wrap(err, "unable to push to remote branch")
	}

	return nil
}
