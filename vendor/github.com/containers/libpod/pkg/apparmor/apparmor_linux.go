// +build linux,apparmor

package apparmor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"text/template"

	"github.com/containers/libpod/pkg/rootless"
	runcaa "github.com/opencontainers/runc/libcontainer/apparmor"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// profileDirectory is the file store for apparmor profiles and macros.
var profileDirectory = "/etc/apparmor.d"

// IsEnabled returns true if AppArmor is enabled on the host.
func IsEnabled() bool {
	if rootless.IsRootless() {
		return false
	}
	return runcaa.IsEnabled()
}

// profileData holds information about the given profile for generation.
type profileData struct {
	// Name is profile name.
	Name string
	// Imports defines the apparmor functions to import, before defining the profile.
	Imports []string
	// InnerImports defines the apparmor functions to import in the profile.
	InnerImports []string
	// Version is the {major, minor, patch} version of apparmor_parser as a single number.
	Version int
}

// generateDefault creates an apparmor profile from ProfileData.
func (p *profileData) generateDefault(out io.Writer) error {
	compiled, err := template.New("apparmor_profile").Parse(libpodProfileTemplate)
	if err != nil {
		return err
	}

	if macroExists("tunables/global") {
		p.Imports = append(p.Imports, "#include <tunables/global>")
	} else {
		p.Imports = append(p.Imports, "@{PROC}=/proc/")
	}

	if macroExists("abstractions/base") {
		p.InnerImports = append(p.InnerImports, "#include <abstractions/base>")
	}

	ver, err := getAAParserVersion()
	if err != nil {
		return err
	}
	p.Version = ver

	return compiled.Execute(out, p)
}

// macrosExists checks if the passed macro exists.
func macroExists(m string) bool {
	_, err := os.Stat(path.Join(profileDirectory, m))
	return err == nil
}

// InstallDefault generates a default profile and loads it into the kernel
// using 'apparmor_parser'.
func InstallDefault(name string) error {
	if rootless.IsRootless() {
		return ErrApparmorRootless
	}

	p := profileData{
		Name: name,
	}

	cmd := exec.Command("apparmor_parser", "-Kr")
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		pipe.Close()
		return err
	}
	if err := p.generateDefault(pipe); err != nil {
		pipe.Close()
		cmd.Wait()
		return err
	}

	pipe.Close()
	return cmd.Wait()
}

// IsLoaded checks if a profile with the given name has been loaded into the
// kernel.
func IsLoaded(name string) (bool, error) {
	if name != "" && rootless.IsRootless() {
		return false, errors.Wrapf(ErrApparmorRootless, "cannot load AppArmor profile %q", name)
	}

	file, err := os.Open("/sys/kernel/security/apparmor/profiles")
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()

	r := bufio.NewReader(file)
	for {
		p, err := r.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return false, err
		}
		if strings.HasPrefix(p, name+" ") {
			return true, nil
		}
	}

	return false, nil
}

// execAAParser runs `apparmor_parser` with the passed arguments.
func execAAParser(dir string, args ...string) (string, error) {
	c := exec.Command("apparmor_parser", args...)
	c.Dir = dir

	output, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("running `%s %s` failed with output: %s\nerror: %v", c.Path, strings.Join(c.Args, " "), output, err)
	}

	return string(output), nil
}

// getAAParserVersion returns the major and minor version of apparmor_parser.
func getAAParserVersion() (int, error) {
	output, err := execAAParser("", "--version")
	if err != nil {
		return -1, err
	}
	return parseAAParserVersion(output)
}

// parseAAParserVersion parses the given `apparmor_parser --version` output and
// returns the major and minor version number as an integer.
func parseAAParserVersion(output string) (int, error) {
	// output is in the form of the following:
	// AppArmor parser version 2.9.1
	// Copyright (C) 1999-2008 Novell Inc.
	// Copyright 2009-2012 Canonical Ltd.
	lines := strings.SplitN(output, "\n", 2)
	words := strings.Split(lines[0], " ")
	version := words[len(words)-1]

	// split by major minor version
	v := strings.Split(version, ".")
	if len(v) == 0 || len(v) > 3 {
		return -1, fmt.Errorf("parsing version failed for output: `%s`", output)
	}

	// Default the versions to 0.
	var majorVersion, minorVersion, patchLevel int

	majorVersion, err := strconv.Atoi(v[0])
	if err != nil {
		return -1, err
	}

	if len(v) > 1 {
		minorVersion, err = strconv.Atoi(v[1])
		if err != nil {
			return -1, err
		}
	}
	if len(v) > 2 {
		patchLevel, err = strconv.Atoi(v[2])
		if err != nil {
			return -1, err
		}
	}

	// major*10^5 + minor*10^3 + patch*10^0
	numericVersion := majorVersion*1e5 + minorVersion*1e3 + patchLevel
	return numericVersion, nil

}

// CheckProfileAndLoadDefault checks if the specified profile is loaded and
// loads the DefaultLibpodProfile if the specified on is prefixed by
// DefaultLipodProfilePrefix.  This allows to always load and apply the latest
// default AppArmor profile.  Note that AppArmor requires root.  If it's a
// default profile, return DefaultLipodProfilePrefix, otherwise the specified
// one.
func CheckProfileAndLoadDefault(name string) (string, error) {
	if name == "unconfined" {
		return name, nil
	}

	if name != "" && rootless.IsRootless() {
		return "", errors.Wrapf(ErrApparmorRootless, "cannot load AppArmor profile %q", name)
	}

	if name != "" && !runcaa.IsEnabled() {
		return "", fmt.Errorf("profile %q specified but AppArmor is disabled on the host", name)
	}

	// If the specified name is not empty or is not a default libpod one,
	// ignore it and return the name.
	if name != "" && !strings.HasPrefix(name, DefaultLipodProfilePrefix) {
		isLoaded, err := IsLoaded(name)
		if err != nil {
			return "", err
		}
		if !isLoaded {
			return "", fmt.Errorf("AppArmor profile %q specified but not loaded")
		}
		return name, nil
	}

	name = DefaultLibpodProfile
	// To avoid expensive redundant loads on each invocation, check
	// if it's loaded before installing it.
	isLoaded, err := IsLoaded(name)
	if err != nil {
		return "", err
	}
	if !isLoaded {
		err = InstallDefault(name)
		if err != nil {
			return "", err
		}
		logrus.Infof("successfully loaded AppAmor profile %q", name)
	} else {
		logrus.Infof("AppAmor profile %q is already loaded", name)
	}

	return name, nil
}
