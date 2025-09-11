package version

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/blang/semver/v4"
	"github.com/containers/common/pkg/apparmor"
	"github.com/containers/common/pkg/seccomp"
	"github.com/google/renameio"
	json "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
)

// Version is the version of the build.
const Version = "1.35.0"

// ReleaseMinorVersions are the currently supported minor versions.
var ReleaseMinorVersions = []string{"1.34", "1.33", "1.32", "1.31"}

// Variables injected during build-time.
var (
	buildDate string // build date in ISO8601 format, output of $(date -u +'%Y-%m-%dT%H:%M:%SZ')
)

type Info struct {
	Version         string   `json:"version,omitempty"`
	GitCommit       string   `json:"gitCommit,omitempty"`
	GitCommitDate   string   `json:"gitCommitDate,omitempty"`
	GitTreeState    string   `json:"gitTreeState,omitempty"`
	BuildDate       string   `json:"buildDate,omitempty"`
	GoVersion       string   `json:"goVersion,omitempty"`
	Compiler        string   `json:"compiler,omitempty"`
	Platform        string   `json:"platform,omitempty"`
	Linkmode        string   `json:"linkmode,omitempty"`
	BuildTags       []string `json:"buildTags,omitempty"`
	LDFlags         string   `json:"ldFlags,omitempty"`
	SeccompEnabled  bool     `json:"seccompEnabled"`
	AppArmorEnabled bool     `json:"appArmorEnabled"`
	Dependencies    []string `json:"dependencies,omitempty"`
}

// ShouldCrioWipe opens the version file, and parses it and the version string
// If there is a parsing error, then crio should wipe, and the error is returned.
// if parsing is successful, it compares the major and minor versions
// and returns whether the major and minor versions are the same.
// If they differ, then crio should wipe.
func ShouldCrioWipe(versionFileName string) (bool, error) {
	return shouldCrioWipe(versionFileName, Version)
}

// shouldCrioWipe is an internal function for testing purposes.
func shouldCrioWipe(versionFileName, versionString string) (bool, error) {
	if versionFileName == "" {
		return false, nil
	}

	versionBytes, err := os.ReadFile(versionFileName)
	if err != nil {
		return true, err
	}

	// parse the version that was laid down by a previous invocation of crio
	var oldVersion semver.Version
	if err := oldVersion.UnmarshalJSON(versionBytes); err != nil {
		return true, fmt.Errorf("version file %s malformatted: %w", versionFileName, err)
	}

	// parse the version of the current binary
	newVersion, err := parseVersionConstant(versionString, "")
	if err != nil {
		return true, fmt.Errorf("version constant %s malformatted: %w", versionString, err)
	}

	// in every case that the minor and major version are out of sync,
	// we want to preform a {down,up}grade. The common case here is newVersion > oldVersion,
	// but even in the opposite case, images are out of date and could be wiped
	return newVersion.Major != oldVersion.Major || newVersion.Minor != oldVersion.Minor, nil
}

// WriteVersionFile writes the version information to a given file is the
// location of the old version file gitCommit is the current git commit
// version. It will be added to the file to aid in debugging, but will not be
// used to compare versions.
func (i *Info) WriteVersionFile(file string) error {
	return writeVersionFile(file, i.GitCommit, Version)
}

// LogVersion logs the version and git information of this build.
func (i *Info) LogVersion() {
	logrus.Infof("Starting CRI-O, version: %s, git: %v(%s)", Version, i.GitCommit, i.GitTreeState)
}

// writeVersionFile is an internal function for testing purposes.
func writeVersionFile(file, gitCommit, version string) error {
	if file == "" {
		return nil
	}

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
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}

	return renameio.WriteFile(file, j, 0o644)
}

// parseVersionConstant parses the Version variable above
// a const crioVersion would be kept, but golang doesn't support
// const structs. We will instead spend some runtime on CRI-O startup
// Because the version string doesn't keep track of the git commit,
// but it could be useful for debugging, we pass it in here
// If our version constant is properly formatted, this should never error.
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

func Get(verbose bool) (*Info, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil, errors.New("unable to retrieve build info")
	}

	const unknown = "unknown"

	gitCommit := unknown
	gitTreeState := "clean"
	gitCommitDate := unknown
	buildTags := []string{}
	ldFlags := unknown

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			gitCommit = s.Value

		case "vcs.modified":
			if s.Value == "true" {
				gitTreeState = "dirty"
			}

		case "vcs.time":
			gitCommitDate = s.Value

		case "-tags":
			buildTags = strings.Split(s.Value, ",")

		case "-ldflags":
			ldFlags = s.Value
		}
	}

	dependencies := []string{}

	if verbose {
		for _, d := range info.Deps {
			dependencies = append(
				dependencies,
				fmt.Sprintf("%s %s %s", d.Path, d.Version, d.Sum),
			)
		}
	}

	return &Info{
		Version:         Version,
		GitCommit:       gitCommit,
		GitCommitDate:   gitCommitDate,
		GitTreeState:    gitTreeState,
		BuildDate:       buildDate,
		GoVersion:       runtime.Version(),
		Compiler:        runtime.Compiler,
		Platform:        fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Linkmode:        linkmode,
		BuildTags:       buildTags,
		LDFlags:         ldFlags,
		SeccompEnabled:  seccomp.IsEnabled(),
		AppArmorEnabled: apparmor.IsEnabled(),
		Dependencies:    dependencies,
	}, nil
}

// String returns the string representation of the version info.
func (i *Info) String() string {
	b := strings.Builder{}
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)

	v := reflect.ValueOf(*i)

	t := v.Type()
	for i := range t.NumField() {
		field := t.Field(i)
		value := v.FieldByName(field.Name)

		valueString := ""
		isMultiLineValue := false

		switch field.Type.Kind() {
		case reflect.Bool:
			valueString = strconv.FormatBool(value.Bool())

		case reflect.Slice:
			// Only expecting []string here; ignore other slices.
			if s, ok := value.Interface().([]string); ok {
				const sep = "\n  "

				valueString = sep + strings.Join(s, sep)
			}

			isMultiLineValue = true

		case reflect.String:
			valueString = value.String()
		}

		if strings.TrimSpace(valueString) != "" {
			fmt.Fprintf(w, "%s:", field.Name)

			if isMultiLineValue {
				fmt.Fprint(w, valueString)
			} else {
				fmt.Fprintf(w, "\t%s", valueString)
			}

			if i+1 < t.NumField() {
				fmt.Fprintf(w, "\n")
			}
		}
	}

	w.Flush()

	return b.String()
}

// JSONString returns the JSON representation of the version info.
func (i *Info) JSONString() (string, error) {
	b, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return "", err
	}

	return string(b), nil
}
