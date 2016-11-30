package apparmor

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/utils/templates"
	"github.com/opencontainers/runc/libcontainer/apparmor"
)

const (
	// defaultApparmorProfile is the name of default apparmor profile name.
	defaultApparmorProfile = "ocid-default"

	// profileDirectory is the file store for apparmor profiles and macros.
	profileDirectory = "/etc/apparmor.d"

	// ContainerAnnotationKeyPrefix is the prefix to an annotation key specifying a container profile.
	ContainerAnnotationKeyPrefix = "container.apparmor.security.beta.kubernetes.io/"

	// ProfileRuntimeDefault is he profile specifying the runtime default.
	ProfileRuntimeDefault = "runtime/default"
	// ProfileNamePrefix is the prefix for specifying profiles loaded on the node.
	ProfileNamePrefix = "localhost/"
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

// InstallDefaultAppArmorProfile installs default apparmor profile.
func InstallDefaultAppArmorProfile() {
	if err := InstallDefault(defaultApparmorProfile); err != nil {
		// Allow daemon to run if loading failed, but are active
		// (possibly through another run, manually, or via system startup)
		if err := IsLoaded(defaultApparmorProfile); err != nil {
			logrus.Errorf("AppArmor enabled on system but the %s profile could not be loaded.", defaultApparmorProfile)
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
	profilePath := f.Name()

	defer f.Close()

	if err := p.generateDefault(f); err != nil {
		return err
	}

	if err := LoadProfile(profilePath); err != nil {
		return err
	}

	return nil
}

// IsLoaded checks if a passed profile has been loaded into the kernel.
func IsLoaded(name string) error {
	file, err := os.Open("/sys/kernel/security/apparmor/profiles")
	if err != nil {
		return err
	}
	defer file.Close()

	r := bufio.NewReader(file)
	for {
		p, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.HasPrefix(p, name+" ") {
			return nil
		}
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

	if err := compiled.Execute(out, p); err != nil {
		return err
	}
	return nil
}

// macrosExists checks if the passed macro exists.
func macroExists(m string) bool {
	_, err := os.Stat(path.Join(profileDirectory, m))
	return err == nil
}
