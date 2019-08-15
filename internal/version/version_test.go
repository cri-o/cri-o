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

func mustUpgrade(t *testing.T, upgrade bool) {
	if !upgrade {
		t.Error("CRI-O should have upgraded")
	}
}

func mustNotUpgrade(t *testing.T, upgrade bool) {
	if upgrade {
		t.Error("CRI-O should not have upgraded")
	}
}

func createTempFile(t *testing.T) *os.File {
	tmpFile, err := ioutil.TempFile("", "temporary-testing-file")
	if err != nil {
		t.Skip()
	}
	return tmpFile
}

func upgradeBetweenVersions(t *testing.T, oldVersion, newVersion string) (bool, error) {
	tmpFile := createTempFile(t)
	defer os.Remove(tmpFile.Name())

	if err := writeVersionFile(tmpFile.Name(), "", oldVersion); err != nil {
		return true, err
	}

	return shouldCrioWipe(tmpFile.Name(), newVersion)
}

func TestParseVersionCorrectVersion(t *testing.T) {
	_, err := parseVersionConstant("1.1.1", "")
	must(t, err)

	_, err = parseVersionConstant("1.1.1-dev", "")
	must(t, err)

	_, err = parseVersionConstant("1.1.1-dev", "biglonggitcommit")
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

func TestParseVersionIgnoresEmptyGitCommit(t *testing.T) {
	gitCommit := ""
	v, err := parseVersionConstant("1.1.1", gitCommit)
	must(t, err)

	// git commit should be included in semver as Build
	if len(v.Build) != 0 {
		t.Error(errors.Errorf("Git commit added despite being empty"))
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
	tmpFile := createTempFile(t)
	defer os.Remove(tmpFile.Name())

	gitCommit := "fakeGitCommit"
	version := "1.1.1"
	err := writeVersionFile(tmpFile.Name(), gitCommit, version)
	must(t, err)

	versionBytes, err := ioutil.ReadFile(tmpFile.Name())
	must(t, err)

	versionConstantVersion, err := parseVersionConstant(version, gitCommit)
	must(t, err)

	versionConstantJSON, err := versionConstantVersion.MarshalJSON()
	must(t, err)
	if string(versionBytes) != string(versionConstantJSON) {
		t.Error(errors.Errorf("Version written is bad. Should be: %s; is: %s", string(versionConstantJSON), string(versionBytes)))
	}
}

func TestWriteVersionFileCreatesDir(t *testing.T) {
	filename := "/tmp/crio/temporary-testing-file"
	err := writeVersionFile(filename, "", "1.1.1")
	must(t, err)

	_, err = ioutil.ReadFile(filename)
	must(t, err)
}

func TestUpgradeWithUnspecifiedVersionFile(t *testing.T) {
	upgrade, err := shouldCrioWipe("", "1.1.1")
	mustUpgrade(t, upgrade)
	fails(t, err)
}

func TestUpgradeWithEmptyVersionFile(t *testing.T) {
	tmpFile := createTempFile(t)
	defer os.Remove(tmpFile.Name())

	upgrade, err := shouldCrioWipe(tmpFile.Name(), "1.1.1")
	mustUpgrade(t, upgrade)
	fails(t, err)
}

func TestFailUpgradeWithFaultyVersionFile(t *testing.T) {
	tmpFile := createTempFile(t)
	defer os.Remove(tmpFile.Name())
	err := ioutil.WriteFile(tmpFile.Name(), []byte("bad version file"), 0644)
	if err != nil {
		t.Skip()
	}

	upgrade, err := shouldCrioWipe(tmpFile.Name(), "1.1.1")
	mustUpgrade(t, upgrade)
	fails(t, err)
}

func TestNoUpgradeWithSameVersion(t *testing.T) {
	upgraded, err := upgradeBetweenVersions(t, "1.1.1", "1.1.1")
	must(t, err)
	mustNotUpgrade(t, upgraded)
}

func TestNoUpgradeWithSubMinorRelease(t *testing.T) {
	upgraded, err := upgradeBetweenVersions(t, "1.1.1", "1.1.2")
	must(t, err)
	mustNotUpgrade(t, upgraded)
}

func TestUpgradeMinorRelease(t *testing.T) {
	upgraded, err := upgradeBetweenVersions(t, "1.14.1", "1.13.1")
	must(t, err)
	mustUpgrade(t, upgraded)
}

func TestUpgradeMajorRelease(t *testing.T) {
	upgraded, err := upgradeBetweenVersions(t, "2.0.0", "1.13.1")
	must(t, err)
	mustUpgrade(t, upgraded)
}

func TestFailNoUpgradeWithBadVersion(t *testing.T) {
	upgraded, err := upgradeBetweenVersions(t, "bad version format", "1.13.1")
	fails(t, err)
	mustUpgrade(t, upgraded)
}
