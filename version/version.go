package version

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/blang/semver"
	"github.com/google/renameio"
)

// Version is the version of the build.
const Version = "1.14.3-dev"

// WriteVersionFile writes the version information to a given file
// file is the location of the old version file
// gitCommit is the current git commit version. It will be added to the file
// to aid in debugging, but will not be used to compare versions
func WriteVersionFile(file, gitCommit string) error {
	current, err := parseVersionConstant(Version, gitCommit)
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
		if err != nil {
			return nil, err
		}
		v.Build = append(v.Build, gitBuild)
	}
	return &v, nil
}
