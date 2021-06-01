package seccomp

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/containers/common/pkg/seccomp"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/cri/types"
	json "github.com/json-iterator/go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	k8sV1 "k8s.io/api/core/v1"
)

// Config is the global seccomp configuration type
type Config struct {
	enabled          bool
	defaultWhenEmpty bool
	profile          *seccomp.Seccomp
}

// New creates a new default seccomp configuration instance
func New() *Config {
	return &Config{
		enabled: seccomp.IsEnabled(),
		profile: seccomp.DefaultProfile(),
	}
}

// Set the seccomp config to use default profile
// when the profile is empty
func (c *Config) SetDefaultWhenEmpty() {
	c.defaultWhenEmpty = true
}

// Returns whether the seccomp config is set to
// use default profile when the profile is empty
func (c *Config) UseDefaultWhenEmpty() bool {
	return c.defaultWhenEmpty
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

		if logrus.IsLevelEnabled(logrus.TraceLevel) {
			profileString, err := json.MarshalToString(c.profile)
			if err != nil {
				return errors.Wrap(err, "marshal default seccomp profile to string")
			}
			logrus.Tracef("Current seccomp profile content: %s", profileString)
		}
		return nil
	}

	profile, err := ioutil.ReadFile(profilePath)
	if err != nil {
		return errors.Wrap(err, "open seccomp profile")
	}

	tmpProfile := &seccomp.Seccomp{}
	if err := json.Unmarshal(profile, tmpProfile); err != nil {
		return errors.Wrap(err, "decoding seccomp profile failed")
	}

	c.profile = tmpProfile
	logrus.Infof("Successfully loaded seccomp profile %q", profilePath)
	logrus.Tracef("Current seccomp profile content: %s", profile)
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

// Setup can be used to setup the seccomp profile.
func (c *Config) Setup(
	ctx context.Context,
	specGenerator *generate.Generator,
	profileField *types.SecurityProfile,
	profilePath string,
) error {
	if profileField == nil {
		// Path based seccomp profiles will be used with a higher priority and are
		// going to be removed in future Kubernetes versions.
		if err := c.setupFromPath(ctx, specGenerator, profilePath); err != nil {
			return errors.Wrap(err, "from profile path")
		}
	} else if err := c.setupFromField(ctx, specGenerator, profileField); err != nil {
		// Field based seccomp profiles are newer than the path based ones and will
		// be the standard in future Kubernetes versions.
		return errors.Wrap(err, "from field")
	}

	return nil
}

func (c *Config) setupFromPath(
	ctx context.Context, specGenerator *generate.Generator, profilePath string,
) error {
	log.Debugf(ctx, "Setup seccomp from profile path: %s", profilePath)

	if profilePath == "" {
		if !c.UseDefaultWhenEmpty() {
			// running w/o seccomp, aka unconfined
			specGenerator.Config.Linux.Seccomp = nil
			return nil
		}
		// default to SeccompProfileRuntimeDefault if user sets UseDefaultWhenEmpty
		profilePath = k8sV1.SeccompProfileRuntimeDefault
	}

	// kubelet defaults sandboxes to run as `runtime/default`, we consider the
	// default profilePath as unconfined if Seccomp disabled
	// https://github.com/kubernetes/kubernetes/blob/12d9183da03d86c65f9f17e3e28be3c7c18ed22a/pkg/kubelet/kuberuntime/kuberuntime_sandbox.go#L162-L163
	if c.IsDisabled() {
		if profilePath == k8sV1.SeccompProfileRuntimeDefault {
			// running w/o seccomp, aka unconfined
			specGenerator.Config.Linux.Seccomp = nil
			return nil
		}
		if profilePath != k8sV1.SeccompProfileNameUnconfined {
			return errors.New(
				"seccomp is not enabled, cannot run with a profile",
			)
		}

		log.Warnf(ctx, "Seccomp is not enabled in the kernel, running container without profile")
	}

	if profilePath == k8sV1.SeccompProfileNameUnconfined {
		// running w/o seccomp, aka unconfined
		specGenerator.Config.Linux.Seccomp = nil
		return nil
	}

	// Load the default seccomp profile from the server if the profilePath is a
	// default one
	if profilePath == k8sV1.SeccompProfileRuntimeDefault || profilePath == k8sV1.DeprecatedSeccompProfileDockerDefault {
		linuxSpecs, err := seccomp.LoadProfileFromConfig(
			c.Profile(), specGenerator.Config,
		)
		if err != nil {
			return errors.Wrap(err, "load default profile")
		}

		specGenerator.Config.Linux.Seccomp = linuxSpecs
		return nil
	}

	// Load local seccomp profiles including their availability validation
	if !strings.HasPrefix(profilePath, k8sV1.SeccompLocalhostProfileNamePrefix) {
		return errors.Errorf("unknown seccomp profile path: %q", profilePath)
	}

	fname := strings.TrimPrefix(profilePath, k8sV1.SeccompLocalhostProfileNamePrefix)
	file, err := ioutil.ReadFile(filepath.FromSlash(fname))
	if err != nil {
		return errors.Errorf("cannot load seccomp profile %q: %v", fname, err)
	}

	linuxSpecs, err := seccomp.LoadProfileFromBytes(file, specGenerator.Config)
	if err != nil {
		return err
	}
	specGenerator.Config.Linux.Seccomp = linuxSpecs
	return nil
}

func (c *Config) setupFromField(
	ctx context.Context,
	specGenerator *generate.Generator,
	profileField *types.SecurityProfile,
) error {
	log.Debugf(ctx, "Setup seccomp from profile field: %+v", profileField)

	if c.IsDisabled() {
		if profileField.ProfileType != types.SecurityProfileTypeUnconfined &&
			// Kubernetes sandboxes run per default with `SecurityProfileTypeRuntimeDefault`:
			// https://github.com/kubernetes/kubernetes/blob/629d5ab/pkg/kubelet/kuberuntime/kuberuntime_sandbox.go#L155-L162
			profileField.ProfileType != types.SecurityProfileTypeRuntimeDefault {
			return errors.Errorf(
				"seccomp is not enabled, cannot run with custom profile",
			)
		}
		log.Warnf(ctx, "Seccomp is not enabled, running without profile")
		specGenerator.Config.Linux.Seccomp = nil
		return nil
	}

	if profileField.ProfileType == types.SecurityProfileTypeUnconfined {
		// running w/o seccomp, aka unconfined
		specGenerator.Config.Linux.Seccomp = nil
		return nil
	}

	if profileField.ProfileType == types.SecurityProfileTypeRuntimeDefault {
		linuxSpecs, err := seccomp.LoadProfileFromConfig(
			c.Profile(), specGenerator.Config,
		)
		if err != nil {
			return errors.Wrap(err, "load default profile")
		}
		specGenerator.Config.Linux.Seccomp = linuxSpecs
		return nil
	}

	// Load local seccomp profiles including their availability validation
	file, err := ioutil.ReadFile(filepath.FromSlash(profileField.LocalhostRef))
	if err != nil {
		return errors.Wrapf(
			err, "unable to load local profile %q", profileField.LocalhostRef,
		)
	}

	linuxSpecs, err := seccomp.LoadProfileFromBytes(file, specGenerator.Config)
	if err != nil {
		return errors.Wrap(err, "load local profile")
	}
	specGenerator.Config.Linux.Seccomp = linuxSpecs
	return nil
}
