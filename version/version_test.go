package version

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/pkg/errors"
)

func must(t *testing.T, err error) {
	if err != nil {
		t.Error(err)
	}
}

func fails(t *testing.T, err error) {
	if err == nil {
		t.Error(err)
	}
}

func TestParseVersionCorrectVersion(t *testing.T) {
	_, err := parseVersionConstant("1.1.1", "")
	must(t, err)

	_, err = parseVersionConstant("1.1.1-dev", "")
	must(t, err)

	_, err = parseVersionConstant("1.1.1-dev", "bigloggitcommit")
	must(t, err)
}

func TestParseVersionAddsGitCommit(t *testing.T) {
	gitCommit := "\"myfavoritecommit\""
	v, err := parseVersionConstant("1.1.1", gitCommit)
	must(t, err)

	// git commit should be included in semver as Build
	if len(v.Build) < 1 {
		t.Error(errors.Errorf("Git commit not included in semver build"))
	}
	// git commit should have quotes removed
	trimmed := strings.Trim(gitCommit, "\"")
	if v.Build[0] != trimmed {
		t.Error(errors.Errorf("Git commit set incorrectly in semver build"))
	}
}

func TestParseVersionBadVersion(t *testing.T) {
	_, err := parseVersionConstant("badversion", "")
	fails(t, err)
}

func TestParseVersionWithCurrentVersion(t *testing.T) {
	_, err := parseVersionConstant(Version, "")
	must(t, err)
}

func TestWriteVersionFileWritesFile(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "temporary-testing-file")
	if err != nil {
		t.Skip()
	}
	defer os.Remove(tmpFile.Name())
	gitCommit := "fakeGitCommit"
	err = WriteVersionFile(tmpFile.Name(), gitCommit)
	must(t, err)

	versionBytes, err := ioutil.ReadFile(tmpFile.Name())
	must(t, err)

	versionConstantVersion, err := parseVersionConstant(Version, gitCommit)
	must(t, err)

	versionConstantJSON, err := versionConstantVersion.MarshalJSON()
	must(t, err)
	if string(versionBytes) != string(versionConstantJSON) {
		t.Error(errors.Errorf("Version written is bad. Should be: %s; is: %s", string(versionConstantJSON), string(versionBytes)))
	}
}

func TestWriteVersionFileCreatesDir(t *testing.T) {
	filename := "/tmp/crio/temporary-testing-file"
	err := WriteVersionFile(filename, "")
	must(t, err)

	_, err = ioutil.ReadFile(filename)
	must(t, err)
}
