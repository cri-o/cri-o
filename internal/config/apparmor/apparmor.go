package apparmor

import (
	"strings"

	"github.com/containers/libpod/pkg/apparmor"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	k8sAppArmor "k8s.io/kubernetes/pkg/security/apparmor"
)

const (
	// DefaultProfile is the default profile name
	DefaultProfile = "crio-default"

	unconfined = "unconfined"
)

// Config is the global AppArmor configuration type
type Config struct {
	enabled        bool
	defaultProfile string
}

// New creates a new default AppArmor configuration instance
func New() *Config {
	return &Config{
		enabled:        apparmor.IsEnabled(),
		defaultProfile: DefaultProfile,
	}
}

// LoadProfile can be used to load a AppArmor profile from the provided path.
// This method will not fail if AppArmor is disabled.
func (c *Config) LoadProfile(profile string) error {
	if !c.IsEnabled() {
		logrus.Info("AppArmor is disabled by the system or at CRI-O build-time")
		return nil
	}

	if profile == unconfined {
		logrus.Info("AppArmor profile is unconfined which basically disables it")
		c.defaultProfile = unconfined
		return nil
	}

	// Load the default profile
	if profile == "" || profile == DefaultProfile {
		logrus.Infof("Installing default AppArmor profile: %v", DefaultProfile)

		if err := apparmor.InstallDefault(DefaultProfile); err != nil {
			return errors.Wrapf(err,
				"installing default AppArmor profile %q failed",
				DefaultProfile,
			)
		}

		if logrus.IsLevelEnabled(logrus.TraceLevel) {
			c, err := apparmor.DefaultContent(DefaultProfile)
			if err != nil {
				return errors.Wrapf(err,
					"retrieving default AppArmor profile %q content failed",
					DefaultProfile,
				)
			}
			logrus.Tracef("Default AppArmor profile contents: %s", c)
		}

		c.defaultProfile = DefaultProfile
		return nil
	}

	// Load a custom profile
	logrus.Infof("Assuming user-provided AppArmor profile: %v", profile)
	isLoaded, err := apparmor.IsLoaded(profile)
	if err != nil {
		return errors.Wrapf(err,
			"checking if AppArmor profile %s is loaded", profile,
		)
	}

	if !isLoaded {
		return errors.Errorf(
			"config provided AppArmor profile %q not loaded", profile,
		)
	}

	c.defaultProfile = profile
	return nil
}

// IsEnabled returns true if AppArmor is enabled via the `apparmor` buildtag
// and globally by the system.
func (c *Config) IsEnabled() bool {
	return c.enabled
}

// Apply returns the trimmed AppArmor profile to be used and reloads if the
// default profile is specified
func (c *Config) Apply(profile string) (string, error) {
	if profile == "" || profile == k8sAppArmor.ProfileRuntimeDefault {
		return c.defaultProfile, nil
	}
	profile = strings.TrimPrefix(profile, k8sAppArmor.ProfileNamePrefix)

	// reload the profile if default
	if profile == DefaultProfile {
		if err := reloadDefaultProfile(); err != nil {
			return "", errors.Wrap(err, "reloading default profile")
		}
	}

	return profile, nil
}

// reloadDefaultProfile reloads the default AppArmor profile and returns an
// error on any failure.
func reloadDefaultProfile() error {
	isLoaded, err := apparmor.IsLoaded(DefaultProfile)
	if err != nil {
		return errors.Wrapf(err,
			"checking if default AppArmor profile %s is loaded", DefaultProfile,
		)
	}
	if !isLoaded {
		if err := apparmor.InstallDefault(DefaultProfile); err != nil {
			return errors.Wrapf(err,
				"installing default AppArmor profile %q failed",
				DefaultProfile,
			)
		}
	}
	return nil
}
