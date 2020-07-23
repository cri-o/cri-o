package seccomp

import (
	"io/ioutil"

	"github.com/pkg/errors"
	json "github.com/pquerna/ffjson/ffjson"
	seccomp "github.com/seccomp/containers-golang"
	"github.com/sirupsen/logrus"
)

// Config is the global seccomp configuration type
type Config struct {
	enabled bool
	profile *seccomp.Seccomp
}

// New creates a new default seccomp configuration instance
func New() *Config {
	return &Config{
		enabled: seccomp.IsEnabled(),
		profile: seccomp.DefaultProfile(),
	}
}

// LoadProfile can be used to load a seccomp profile from the provided path.
// This method will not fail if seccomp is disabled.
func (c *Config) LoadProfile(profilePath string) error {
	if c.IsDisabled() {
		logrus.Info("Seccomp is disabled by the system or at CRI-O build-time")
		return nil
	}

	if profilePath == "" {
		c.profile = seccomp.DefaultProfile()
		logrus.Info("No seccomp profile specified, using the internal default")
		logrus.Debugf("Current seccomp profile content: %+v", c.profile)
		return nil
	}

	profile, err := ioutil.ReadFile(profilePath)
	if err != nil {
		return errors.Wrapf(err, "open seccomp profile %s failed", profilePath)
	}

	tmpProfile := &seccomp.Seccomp{}
	if err := json.Unmarshal(profile, tmpProfile); err != nil {
		return errors.Wrap(err, "decoding seccomp profile failed")
	}

	c.profile = tmpProfile
	logrus.Infof("Successfully loaded seccomp profile %q", profilePath)
	logrus.Debugf("Current seccomp profile content: %+v", c.profile)
	return nil
}

// IsDisabled returns true if seccomp is disabled either via the missing
// `seccomp` buildtag or globally by the system.
func (c *Config) IsDisabled() bool {
	return !c.enabled
}

// Profile returns the currently loaded seccomp profile
func (c *Config) Profile() *seccomp.Seccomp {
	return c.profile
}
