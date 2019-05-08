package version

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/blang/semver"
	"github.com/google/renameio"
)

// Version is the version of the build.
const Version = "1.14.1-dev"

// WriteVersionFile writes the version information to a given file
// file is the location of the old version file
// gitCommit is the current git commit version. It will be added to the file
// to aid in debugging, but will not be used to compare versions
func WriteVersionFile(file string, gitCommit string) error {
	current, err := parseVersionConstant(gitCommit)
	// Sanity check-this should never happen
	if err != nil {
		return err
	}
	json, err := current.MarshalJSON()
	// Sanity check-this should never happen
	if err != nil {
		return err
	}

	return renameio.WriteFile(file, json, 0644)
}

// ShouldCrioUpgrade returns whether the currentVerison is
// a version that is far enough in the future from the pastVersion
// to warrant upgrading the node.
// file is the location of the old version file
func ShouldCrioUpgrade(file string) (bool, error) {
	old, err := parseVersionFile(file)
	if err != nil {
		// If the file doesn't exist, an upgrade should happen.
		// This is an expected case and thus not an error
		if os.IsNotExist(err) {
			return true, nil
		}
		// If our error is something else, we probably should upgrade
		// but let's pass along the error so it can be dealt with
		// by the caller
		return true, err
	}

	// gitCommit is currently not used to check whether crio should upgrade
	current, err := parseVersionConstant("")
	// Sanity check-this should never happen
	if err != nil {
		return true, err
	}

	if old.Major < current.Major {
		return true, nil
	}
	if old.Major > current.Major {
		return false, nil
	}

	if old.Minor < current.Minor {
		return true, nil
	}
	// As of now, we only care about major and minor versions
	// If these aren't out of date, no need to upgrade
	return false, nil
}

// parseVersionConstant parses the Version variable above
// a const crioVersion would be kept, but golang doesn't support
// const structs. We will instead spend some runtime on CRI-O startup
// Because the version string doesn't keep track of the git commit,
// but it could be useful for debugging, we pass it in here
// If our version constant is properly formatted, this should never error
func parseVersionConstant(gitCommit string) (*semver.Version, error) {
	v, err := semver.Make(Version)
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

// parseVersionFile reads a given version.json file for the previous version
func parseVersionFile(file string) (*semver.Version, error) {
	vFile, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	vBytes, err := ioutil.ReadAll(vFile)
	if err != nil {
		return nil, err
	}

	var v semver.Version
	err = v.UnmarshalJSON(vBytes)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
