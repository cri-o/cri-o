//go:build linux && cgo
// +build linux,cgo

package seccomp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/containers/common/pkg/seccomp"
	"github.com/cri-o/cri-o/internal/log"
	json "github.com/json-iterator/go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	k8sV1 "k8s.io/api/core/v1"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var (
	defaultProfileOnce sync.Once
	defaultProfile     *seccomp.Seccomp
)

// DefaultProfile is used to allow mutations from the DefaultProfile from the seccomp library.
// Specifically, it is used to filter `unshare` from the default profile, as it is a risky syscall for unprivileged containers
// to have access to.
func DefaultProfile() *seccomp.Seccomp {
	defaultProfileOnce.Do(func() {
		const (
			unshareName              = "unshare"
			unshareParentStructIndex = 1
			unshareIndex             = 363
		)
		prof := seccomp.DefaultProfile()
		// We know the default profile at compile time
		// though a vendor change may update it.
		// Panic on error and have CI catch errors on vendor bumps,
		// to avoid combing through.
		if prof.Syscalls[unshareParentStructIndex].Names[unshareIndex] != unshareName {
			for i, name := range prof.Syscalls[unshareParentStructIndex].Names {
				if name == unshareName {
					_, file, _, _ := runtime.Caller(1)
					logrus.Errorf("Change the `unshareIndex` variable in %s to %d", file, i)
					break
				}
			}
			logrus.Fatalf(
				"Default seccomp profile updated and unshare syscall moved. Found unexpected syscall: %q",
				prof.Syscalls[unshareParentStructIndex].Names[unshareIndex],
			)
		}
		removeStringFromSlice(prof.Syscalls[unshareParentStructIndex].Names, unshareIndex)

		prof.Syscalls = append(prof.Syscalls, &seccomp.Syscall{
			Names: []string{
				unshareName,
			},
			Action: seccomp.ActAllow,
			Includes: seccomp.Filter{
				Caps: []string{"CAP_SYS_ADMIN"},
			},
		})
		defaultProfile = prof
	})

	return defaultProfile
}

func removeStringFromSlice(s []string, i int) []string {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

// Config is the global seccomp configuration type
type Config struct {
	enabled          bool
	defaultWhenEmpty bool
	profile          *seccomp.Seccomp
	notifierPath     string
}

// New creates a new default seccomp configuration instance
func New() *Config {
	return &Config{
		enabled:          seccomp.IsEnabled(),
		profile:          DefaultProfile(),
		defaultWhenEmpty: true,
		notifierPath:     "/var/run/crio/seccomp",
	}
}

// SetUseDefaultWhenEmpty uses the default seccomp profile if true is passed as
// argument, otherwise unconfined.
func (c *Config) SetUseDefaultWhenEmpty(to bool) {
	logrus.Infof("Using seccomp default profile when unspecified: %v", to)
	c.defaultWhenEmpty = to
}

// Returns whether the seccomp config is set to
// use default profile when the profile is empty
func (c *Config) UseDefaultWhenEmpty() bool {
	return c.defaultWhenEmpty
}

// SetNotifierPath sets the default path for creating seccomp notifier sockets.
func (c *Config) SetNotifierPath(path string) {
	c.notifierPath = path
}

// NotifierPath returns the currently used seccomp notifier base path.
func (c *Config) NotifierPath() string {
	return c.notifierPath
}

// LoadProfile can be used to load a seccomp profile from the provided path.
// This method will not fail if seccomp is disabled.
func (c *Config) LoadProfile(profilePath string) error {
	if c.IsDisabled() {
		logrus.Info("Seccomp is disabled by the system or at CRI-O build-time")
		return nil
	}

	if profilePath == "" {
		if err := c.LoadDefaultProfile(); err != nil {
			return fmt.Errorf("load default seccomp profile: %w", err)
		}
		return nil
	}

	profile, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("open seccomp profile: %w", err)
	}

	tmpProfile := &seccomp.Seccomp{}
	if err := json.Unmarshal(profile, tmpProfile); err != nil {
		return fmt.Errorf("decoding seccomp profile failed: %w", err)
	}

	c.profile = tmpProfile
	logrus.Infof("Successfully loaded seccomp profile %q", profilePath)
	logrus.Tracef("Current seccomp profile content: %s", profile)
	return nil
}

// LoadDefaultProfile sets the internal default profile.
func (c *Config) LoadDefaultProfile() error {
	logrus.Info("Using the internal default seccomp profile")
	c.profile = DefaultProfile()

	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		profileString, err := json.MarshalToString(c.profile)
		if err != nil {
			return fmt.Errorf("marshal default seccomp profile to string: %w", err)
		}
		logrus.Tracef("Default seccomp profile content: %s", profileString)
	}

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
	msgChan chan Notification,
	containerID string,
	annotations map[string]string,
	specGenerator *generate.Generator,
	profileField *types.SecurityProfile,
	profilePath string,
) (notifier *Notifier, err error) {
	if profileField == nil {
		// Path based seccomp profiles will be used with a higher priority and are
		// going to be removed in future Kubernetes versions.
		notifier, err = c.setupFromPath(ctx, msgChan, containerID, annotations, specGenerator, profilePath)
		if err != nil {
			return nil, fmt.Errorf("from profile path: %w", err)
		}
	} else {
		notifier, err = c.setupFromField(ctx, msgChan, containerID, annotations, specGenerator, profileField)
		if err != nil {
			// Field based seccomp profiles are newer than the path based ones and will
			// be the standard in future Kubernetes versions.
			return nil, fmt.Errorf("from field: %w", err)
		}
	}

	return notifier, nil
}

func (c *Config) setupFromPath(
	ctx context.Context,
	msgChan chan Notification,
	containerID string,
	annotations map[string]string,
	specGenerator *generate.Generator,
	profilePath string,
) (*Notifier, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Debugf(ctx, "Setup seccomp from profile path: %s", profilePath)

	if profilePath == "" {
		if !c.UseDefaultWhenEmpty() {
			// running w/o seccomp, aka unconfined
			specGenerator.Config.Linux.Seccomp = nil
			return nil, nil
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
			return nil, nil
		}
		if profilePath != k8sV1.SeccompProfileNameUnconfined {
			return nil, errors.New(
				"seccomp is not enabled, cannot run with a profile",
			)
		}

		log.Warnf(ctx, "Seccomp is not enabled in the kernel, running container without profile")
	}

	if profilePath == k8sV1.SeccompProfileNameUnconfined {
		// running w/o seccomp, aka unconfined
		specGenerator.Config.Linux.Seccomp = nil
		return nil, nil
	}

	// Load the default seccomp profile from the server if the profilePath is a
	// default one
	if profilePath == k8sV1.SeccompProfileRuntimeDefault || profilePath == k8sV1.DeprecatedSeccompProfileDockerDefault {
		linuxSpecs, err := seccomp.LoadProfileFromConfig(
			c.Profile(), specGenerator.Config,
		)
		if err != nil {
			return nil, fmt.Errorf("load default profile: %w", err)
		}

		notifier, err := c.injectNotifier(ctx, msgChan, containerID, annotations, linuxSpecs)
		if err != nil {
			return nil, fmt.Errorf("inject notifier: %w", err)
		}
		specGenerator.Config.Linux.Seccomp = linuxSpecs
		return notifier, nil
	}

	// Load local seccomp profiles including their availability validation
	if !strings.HasPrefix(profilePath, k8sV1.SeccompLocalhostProfileNamePrefix) {
		return nil, fmt.Errorf("unknown seccomp profile path: %q", profilePath)
	}

	fname := strings.TrimPrefix(profilePath, k8sV1.SeccompLocalhostProfileNamePrefix)
	file, err := os.ReadFile(filepath.FromSlash(fname))
	if err != nil {
		return nil, fmt.Errorf("cannot load seccomp profile %q: %w", fname, err)
	}

	linuxSpecs, err := seccomp.LoadProfileFromBytes(file, specGenerator.Config)
	if err != nil {
		return nil, err
	}

	notifier, err := c.injectNotifier(ctx, msgChan, containerID, annotations, linuxSpecs)
	if err != nil {
		return nil, fmt.Errorf("inject notifier: %w", err)
	}
	specGenerator.Config.Linux.Seccomp = linuxSpecs

	return notifier, nil
}

func (c *Config) setupFromField(
	ctx context.Context,
	msgChan chan Notification,
	containerID string,
	annotations map[string]string,
	specGenerator *generate.Generator,
	profileField *types.SecurityProfile,
) (*Notifier, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Debugf(ctx, "Setup seccomp from profile field: %+v", profileField)

	if c.IsDisabled() {
		if profileField.ProfileType != types.SecurityProfile_Unconfined &&
			// Kubernetes sandboxes run per default with `SecurityProfileTypeRuntimeDefault`:
			// https://github.com/kubernetes/kubernetes/blob/629d5ab/pkg/kubelet/kuberuntime/kuberuntime_sandbox.go#L155-L162
			profileField.ProfileType != types.SecurityProfile_RuntimeDefault {
			return nil, errors.New(
				"seccomp is not enabled, cannot run with custom profile",
			)
		}
		log.Warnf(ctx, "Seccomp is not enabled, running without profile")
		specGenerator.Config.Linux.Seccomp = nil
		return nil, nil
	}

	if profileField.ProfileType == types.SecurityProfile_Unconfined {
		// running w/o seccomp, aka unconfined
		specGenerator.Config.Linux.Seccomp = nil
		return nil, nil
	}

	if profileField.ProfileType == types.SecurityProfile_RuntimeDefault {
		linuxSpecs, err := seccomp.LoadProfileFromConfig(
			c.Profile(), specGenerator.Config,
		)
		if err != nil {
			return nil, fmt.Errorf("load default profile: %w", err)
		}
		notifier, err := c.injectNotifier(ctx, msgChan, containerID, annotations, linuxSpecs)
		if err != nil {
			return nil, fmt.Errorf("inject notifier: %w", err)
		}
		specGenerator.Config.Linux.Seccomp = linuxSpecs
		return notifier, nil
	}

	// Load local seccomp profiles including their availability validation
	file, err := os.ReadFile(filepath.FromSlash(profileField.LocalhostRef))
	if err != nil {
		return nil, fmt.Errorf(
			"unable to load local profile %q: %w", profileField.LocalhostRef, err,
		)
	}

	linuxSpecs, err := seccomp.LoadProfileFromBytes(file, specGenerator.Config)
	if err != nil {
		return nil, fmt.Errorf("load local profile: %w", err)
	}
	notifier, err := c.injectNotifier(ctx, msgChan, containerID, annotations, linuxSpecs)
	if err != nil {
		return nil, fmt.Errorf("inject notifier: %w", err)
	}
	specGenerator.Config.Linux.Seccomp = linuxSpecs
	return notifier, nil
}
