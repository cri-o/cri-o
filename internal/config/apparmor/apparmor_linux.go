package apparmor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"go.podman.io/common/pkg/apparmor"
	v1 "k8s.io/api/core/v1"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// DefaultProfile is the default profile name.
const DefaultProfile = "crio-default"

// Config is the global AppArmor configuration type.
type Config struct {
	enabled        bool
	defaultProfile string
}

// New creates a new default AppArmor configuration instance.
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

	if profile == v1.DeprecatedAppArmorBetaProfileNameUnconfined {
		logrus.Info("AppArmor profile is unconfined which basically disables it")

		c.defaultProfile = v1.DeprecatedAppArmorBetaProfileNameUnconfined

		return nil
	}

	// Load the default profile
	if profile == "" || profile == DefaultProfile {
		logrus.Infof("Installing default AppArmor profile: %v", DefaultProfile)

		if err := apparmor.InstallDefault(DefaultProfile); err != nil {
			return fmt.Errorf(
				"installing default AppArmor profile %q failed: %w",
				DefaultProfile, err,
			)
		}

		if logrus.IsLevelEnabled(logrus.TraceLevel) {
			c, err := apparmor.DefaultContent(DefaultProfile)
			if err != nil {
				return fmt.Errorf(
					"retrieving default AppArmor profile %q content failed: %w",
					DefaultProfile, err,
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
		return fmt.Errorf(
			"checking if AppArmor profile %s is loaded: %w", profile, err,
		)
	}

	if !isLoaded {
		return fmt.Errorf(
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
// default profile is specified.
// The AppArmor profile to the CRI via the deprecated apparmor_profile field
// in favor of the newer structured apparmor field.
// CRI provides the AppArmor profile via both fields to maintain backwards compatibility.
// ref https://github.com/kubernetes/kubernetes/pull/123811
// Process new field and fallback to deprecated. From the kubernetes side both fields are populated.
// TODO: Clean off deprecated AppArmorProfile usage.
func (c *Config) Apply(p *runtimeapi.LinuxContainerSecurityContext) (string, error) {
	// Runtime default profile
	if p.GetApparmor() != nil && p.GetApparmor().GetProfileType() == runtimeapi.SecurityProfile_RuntimeDefault {
		return c.defaultProfile, nil
	}

	//nolint:staticcheck // see deprecation TODO above
	if p.GetApparmor() == nil && p.GetApparmorProfile() == "" || p.GetApparmorProfile() == v1.DeprecatedAppArmorBetaProfileRuntimeDefault {
		return c.defaultProfile, nil
	}

	securityProfile := ""
	//nolint:staticcheck // see deprecation TODO above
	if p.GetApparmor() == nil && p.GetApparmorProfile() != "" {
		securityProfile = p.GetApparmorProfile()
	}

	if p.GetApparmor() != nil && p.GetApparmor().GetLocalhostRef() != "" {
		securityProfile = p.GetApparmor().GetLocalhostRef()
	}

	//nolint:staticcheck // see deprecation TODO above
	if p.GetApparmor() == nil && strings.EqualFold(p.GetApparmorProfile(), v1.DeprecatedAppArmorBetaProfileNameUnconfined) {
		securityProfile = v1.DeprecatedAppArmorBetaProfileNameUnconfined
	}

	if p.GetApparmor() != nil && strings.EqualFold(p.GetApparmor().GetProfileType().String(), v1.DeprecatedAppArmorBetaProfileNameUnconfined) {
		securityProfile = v1.DeprecatedAppArmorBetaProfileNameUnconfined
	}

	securityProfile = strings.TrimPrefix(securityProfile, v1.DeprecatedAppArmorBetaProfileNamePrefix)
	if securityProfile == "" {
		return "", errors.New("empty localhost AppArmor profile is forbidden")
	}

	if securityProfile == DefaultProfile {
		if err := reloadDefaultProfile(); err != nil {
			return "", fmt.Errorf("reloading default profile: %w", err)
		}
	}

	return securityProfile, nil
}

// reloadDefaultProfile reloads the default AppArmor profile and returns an
// error on any failure.
func reloadDefaultProfile() error {
	isLoaded, err := apparmor.IsLoaded(DefaultProfile)
	if err != nil {
		return fmt.Errorf(
			"checking if default AppArmor profile %s is loaded: %w", DefaultProfile, err,
		)
	}

	if !isLoaded {
		if err := apparmor.InstallDefault(DefaultProfile); err != nil {
			return fmt.Errorf(
				"installing default AppArmor profile %q failed: %w",
				DefaultProfile, err,
			)
		}
	}

	return nil
}
