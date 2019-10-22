package version

import (
	"bufio"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/blang/semver"
	"github.com/google/renameio"
	"github.com/pkg/errors"
)

// Version is the version of the build.
const Version = "1.17.0-dev"

// ShouldCrioWipe opens the version file, and parses it and the version string
// If there is a parsing error, then crio should wipe, and the error is returned.
// if parsing is successful, it compares the major and minor versions
// and returns whether the major and minor versions are the same.
// If they differ, then crio should wipe.
func ShouldCrioWipe(versionFileName string) (bool, error) {
	return shouldCrioWipe(versionFileName, Version)
}

// shouldCrioWipe is an internal function for testing purposes
func shouldCrioWipe(versionFileName, versionString string) (bool, error) {
	f, err := os.Open(versionFileName)
	if err != nil {
		return true, errors.Errorf("version file %s not found: %v. Triggering wipe", versionFileName, err)
	}
	r := bufio.NewReader(f)
	versionBytes, err := ioutil.ReadAll(r)
	if err != nil {
		return true, errors.Errorf("reading version file %s failed: %v. Triggering wipe", versionFileName, err)
	}

	// parse the version that was laid down by a previous invocation of crio
	var oldVersion semver.Version
	if err := oldVersion.UnmarshalJSON(versionBytes); err != nil {
		return true, errors.Errorf("version file %s malformatted: %v. Triggering wipe", versionFileName, err)
	}

	// parse the version of the current binary
	newVersion, err := parseVersionConstant(versionString, "")
	if err != nil {
		return true, errors.Errorf("version constant %s malformatted: %v. Triggering wipe", versionString, err)
	}

	// in every case that the minor and major version are out of sync,
	// we want to preform a {down,up}grade. The common case here is newVersion > oldVersion,
	// but even in the opposite case, images are out of date and could be wiped
	return newVersion.Major != oldVersion.Major || newVersion.Minor != oldVersion.Minor, nil
}

// WriteVersionFile writes the version information to a given file
// file is the location of the old version file
// gitCommit is the current git commit version. It will be added to the file
// to aid in debugging, but will not be used to compare versions
func WriteVersionFile(file, gitCommit string) error {
	return writeVersionFile(file, gitCommit, Version)
}

// writeVersionFile is an internal function for testing purposes
func writeVersionFile(file, gitCommit, version string) error {
	current, err := parseVersionConstant(version, gitCommit)
	// Sanity check-this should never happen
	if err != nil {
		return err
	}
	json, err := current.MarshalJSON()
	// Sanity check-this should never happen
	if err != nil {
		return err
	}

	// Create the top level directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}

	return renameio.WriteFile(file, json, 0644)
}

// parseVersionConstant parses the Version variable above
// a const crioVersion would be kept, but golang doesn't support
// const structs. We will instead spend some runtime on CRI-O startup
// Because the version string doesn't keep track of the git commit,
// but it could be useful for debugging, we pass it in here
// If our version constant is properly formatted, this should never error
func parseVersionConstant(versionString, gitCommit string) (*semver.Version, error) {
	v, err := semver.Make(versionString)
	if err != nil {
		return nil, err
	}
	if gitCommit != "" {
		gitBuild, err := semver.NewBuildVersion(strings.Trim(gitCommit, "\""))
		// If gitCommit is empty, silently error, as it's helpful, but not needed.
		if err == nil {
			v.Build = append(v.Build, gitBuild)
		}
	}
	return &v, nil
}
