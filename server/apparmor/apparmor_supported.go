// +build apparmor

package apparmor

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/utils/templates"
	"github.com/opencontainers/runc/libcontainer/apparmor"
)

const (
	// profileDirectory is the file store for apparmor profiles and macros.
	profileDirectory = "/etc/apparmor.d"

	// readConfigTimeout is the timeout of reading apparmor profiles.
	readConfigTimeout = 10
)

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

// LoadDefaultAppArmorProfile loads default apparmor profile, if it is not loaded.
func LoadDefaultAppArmorProfile() {
	if !IsLoaded(DefaultApparmorProfile) {
		if err := InstallDefault(DefaultApparmorProfile); err != nil {
			logrus.Errorf("AppArmor enabled on system but the %s profile could not be loaded:%v", DefaultApparmorProfile, err)
		}
	}
}

// IsEnabled returns true if apparmor is enabled for the host.
func IsEnabled() bool {
	return apparmor.IsEnabled()
}

// GetProfileNameFromPodAnnotations gets the name of the profile to use with container from
// pod annotations
func GetProfileNameFromPodAnnotations(annotations map[string]string, containerName string) string {
	return annotations[ContainerAnnotationKeyPrefix+containerName]
}

// InstallDefault generates a default profile in a temp directory determined by
// os.TempDir(), then loads the profile into the kernel using 'apparmor_parser'.
func InstallDefault(name string) error {
	p := profileData{
		Name: name,
	}

	// Install to a temporary directory.
	f, err := ioutil.TempFile("", name)
	if err != nil {
		return err
	}
	defer f.Close()

	if err := p.generateDefault(f); err != nil {
		return err
	}

	return LoadProfile(f.Name())
}

// IsLoaded checks if a passed profile has been loaded into the kernel.
func IsLoaded(name string) bool {
	file, err := os.Open("/sys/kernel/security/apparmor/profiles")
	if err != nil {
		return false
	}
	defer file.Close()

	ch := make(chan bool, 1)

	go func() {
		r := bufio.NewReader(file)
		for {
			p, err := r.ReadString('\n')
			if err != nil {
				ch <- false
			}
			if strings.HasPrefix(p, name+" ") {
				ch <- true
			}
		}
	}()

	select {
	case <-time.After(time.Duration(readConfigTimeout) * time.Second):
		return false
	case enabled := <-ch:
		return enabled
	}
}

// generateDefault creates an apparmor profile from ProfileData.
func (p *profileData) generateDefault(out io.Writer) error {
	compiled, err := templates.NewParse("apparmor_profile", baseTemplate)
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

	ver, err := GetVersion()
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
