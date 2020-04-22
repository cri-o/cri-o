package version

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/blang/semver"
	"github.com/google/renameio"
	"github.com/pkg/errors"

	"github.com/cri-o/cri-o/utils"
)

// Variables injected during build-time
var (
	version      string // Version is the version of the build.
	gitCommit    string // sha1 from git, output of $(git rev-parse HEAD)
	gitTreeState string // state of git tree, either "clean" or "dirty"
	buildDate    string // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
)

type Info struct {
	Version      string `json:"version,omitempty"`
	GitCommit    string `json:"gitCommit,omitempty"`
	GitTreeState string `json:"gitTreeState,omitempty"`
	BuildDate    string `json:"buildDate,omitempty"`
	GoVersion    string `json:"goVersion,omitempty"`
	Compiler     string `json:"compiler,omitempty"`
	Platform     string `json:"platform,omitempty"`
	Linkmode     string `json:"linkmode,omitempty"`
}

// ShouldCrioWipe opens the version file, and parses it and the version string
// If there is a parsing error, then crio should wipe, and the error is returned.
// if parsing is successful, it compares the major and minor versions
// and returns whether the major and minor versions are the same.
// If they differ, then crio should wipe.
func ShouldCrioWipe(versionFileName string) (bool, error) {
	return shouldCrioWipe(versionFileName, version)
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
func WriteVersionFile(file string) error {
	return writeVersionFile(file, gitCommit, version)
}

// writeVersionFile is an internal function for testing purposes
func writeVersionFile(file, gitCommit, version string) error {
	current, err := parseVersionConstant(version, gitCommit)
	// Sanity check-this should never happen
	if err != nil {
		return err
	}
	j, err := current.MarshalJSON()
	// Sanity check-this should never happen
	if err != nil {
		return err
	}

	// Create the top level directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}

	return renameio.WriteFile(file, j, 0644)
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

func Get() *Info {
	return &Info{
		Version:      version,
		GitCommit:    gitCommit,
		GitTreeState: gitTreeState,
		BuildDate:    buildDate,
		GoVersion:    runtime.Version(),
		Compiler:     runtime.Compiler,
		Platform:     fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Linkmode:     getLinkmode(),
	}
}

// String returns the string representation of the version info
func (i *Info) String() string {
	b := strings.Builder{}
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)

	v := reflect.ValueOf(*i)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.FieldByName(field.Name).String()
		fmt.Fprintf(w, "%s:\t%s", field.Name, value)
		if i+1 < t.NumField() {
			fmt.Fprintf(w, "\n")
		}
	}

	w.Flush()
	return b.String()
}

func getLinkmode() string {
	abspath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return fmt.Sprintf("unknown: %v", err)
	}

	output, err := utils.ExecCmd("ldd", abspath)
	if err != nil {
		return fmt.Sprintf("unknown: %v", err)
	}

	if strings.Contains(output, "not a dynamic executable") {
		return "static"
	}

	return "dynamic"
}

// JSONString returns the JSON representation of the version info
func (i *Info) JSONString() (string, error) {
	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
