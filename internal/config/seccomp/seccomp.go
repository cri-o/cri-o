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
	"sync"

	"github.com/containers/common/pkg/seccomp"
	"github.com/cri-o/cri-o/internal/log"
	json "github.com/json-iterator/go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var (
	defaultProfileOnce sync.Once
	defaultProfile     *seccomp.Seccomp
)

// DefaultProfile is used to allow mutations from the DefaultProfile from the seccomp library.
// Specifically, it is used to filter syscalls which can create namespaces from the default
// profile, as it is risky for unprivileged containers to have access to create Linux
// namespaces.
func DefaultProfile() *seccomp.Seccomp {
	defaultProfileOnce.Do(func() {
		removeSyscalls := []struct {
			Name              string
			ParentStructIndex int
			Index             int
		}{
			{"clone", 1, 23},
			{"clone3", 1, 24},
			{"unshare", 1, 363},
		}

		prof := seccomp.DefaultProfile()
		// We know the default profile at compile time
		// though a vendor change may update it.
		// Panic on error and have CI catch errors on vendor bumps,
		// to avoid combing through.
		for _, remove := range removeSyscalls {
			if prof.Syscalls[remove.ParentStructIndex].Names[remove.Index] != remove.Name {
				for i, name := range prof.Syscalls[remove.ParentStructIndex].Names {
					if name == remove.Name {
						_, file, _, _ := runtime.Caller(1)
						logrus.Errorf("Change the Index for %q in %s to %d", remove.Name, file, i)
						break
					}
				}
				logrus.Fatalf(
					"Default seccomp profile updated and syscall moved. Found unexpected syscall: %q",
					prof.Syscalls[remove.ParentStructIndex].Names[remove.Index],
				)
			}
			removeStringFromSlice(prof.Syscalls[remove.ParentStructIndex].Names, remove.Index)
		}

		prof.Syscalls = append(prof.Syscalls, &seccomp.Syscall{
			Names: []string{
				"clone",
				"clone3",
				"unshare",
			},
			Action: seccomp.ActAllow,
			Includes: seccomp.Filter{
				Caps: []string{"CAP_SYS_ADMIN"},
			},
		})

		var flagsIndex uint = 0
		if runtime.GOARCH == "s390" || runtime.GOARCH == "s390x" {
			flagsIndex = 1
		}

		prof.Syscalls = append(prof.Syscalls, &seccomp.Syscall{
			Names: []string{
				"clone",
			},
			Action: seccomp.ActAllow,
			Args: []*seccomp.Arg{
				{
					Index:    flagsIndex,
					Value:    unix.CLONE_NEWNS | unix.CLONE_NEWUTS | unix.CLONE_NEWIPC | unix.CLONE_NEWUSER | unix.CLONE_NEWPID | unix.CLONE_NEWNET | unix.CLONE_NEWCGROUP,
					ValueTwo: 0,
					Op:       seccomp.OpMaskedEqual,
				},
			},
		},
			&seccomp.Syscall{
				Names: []string{
					"clone",
				},
				Action: seccomp.ActErrno,
				Errno:  "EPERM",
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
) (*Notifier, string, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Debugf(ctx, "Setup seccomp from profile field: %+v", profileField)

	if profileField == nil {
		if !c.UseDefaultWhenEmpty() {
			// running w/o seccomp, aka unconfined
			specGenerator.Config.Linux.Seccomp = nil
			return nil, "", nil
		}

		profileField = &types.SecurityProfile{
			ProfileType: types.SecurityProfile_RuntimeDefault,
		}
	}

	if c.IsDisabled() {
		if profileField.ProfileType != types.SecurityProfile_Unconfined &&
			// Kubernetes sandboxes run per default with `SecurityProfileTypeRuntimeDefault`:
			// https://github.com/kubernetes/kubernetes/blob/629d5ab/pkg/kubelet/kuberuntime/kuberuntime_sandbox.go#L155-L162
			profileField.ProfileType != types.SecurityProfile_RuntimeDefault {
			return nil, "", errors.New(
				"seccomp is not enabled, cannot run with custom profile",
			)
		}
		log.Warnf(ctx, "Seccomp is not enabled, running without profile")
		specGenerator.Config.Linux.Seccomp = nil
		return nil, types.SecurityProfile_Unconfined.String(), nil
	}

	if profileField.ProfileType == types.SecurityProfile_Unconfined {
		// running w/o seccomp, aka unconfined
		specGenerator.Config.Linux.Seccomp = nil
		return nil, types.SecurityProfile_Unconfined.String(), nil
	}

	if profileField.ProfileType == types.SecurityProfile_RuntimeDefault {
		linuxSpecs, err := seccomp.LoadProfileFromConfig(
			c.Profile(), specGenerator.Config,
		)
		if err != nil {
			return nil, "", fmt.Errorf("load default profile: %w", err)
		}
		notifier, err := c.injectNotifier(ctx, msgChan, containerID, annotations, linuxSpecs)
		if err != nil {
			return nil, "", fmt.Errorf("inject notifier: %w", err)
		}
		specGenerator.Config.Linux.Seccomp = linuxSpecs
		return notifier, types.SecurityProfile_RuntimeDefault.String(), nil
	}

	// Load local seccomp profiles including their availability validation
	localhostRef := filepath.FromSlash(profileField.LocalhostRef)
	file, err := os.ReadFile(localhostRef)
	if err != nil {
		return nil, "", fmt.Errorf(
			"unable to load local profile %q: %w", localhostRef, err,
		)
	}

	linuxSpecs, err := seccomp.LoadProfileFromBytes(file, specGenerator.Config)
	if err != nil {
		return nil, "", fmt.Errorf("load local profile: %w", err)
	}
	notifier, err := c.injectNotifier(ctx, msgChan, containerID, annotations, linuxSpecs)
	if err != nil {
		return nil, "", fmt.Errorf("inject notifier: %w", err)
	}
	specGenerator.Config.Linux.Seccomp = linuxSpecs
	return notifier, localhostRef, nil
}
