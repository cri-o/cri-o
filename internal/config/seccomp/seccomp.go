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
	imagetypes "github.com/containers/image/v5/types"
	json "github.com/json-iterator/go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/config/seccomp/seccompociartifact"
	"github.com/cri-o/cri-o/internal/log"
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
			unshareIndex             = 364
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

// Config is the global seccomp configuration type.
type Config struct {
	enabled      bool
	profile      *seccomp.Seccomp
	notifierPath string
}

// New creates a new default seccomp configuration instance.
func New() *Config {
	return &Config{
		enabled:      seccomp.IsEnabled(),
		profile:      DefaultProfile(),
		notifierPath: "/var/run/crio/seccomp",
	}
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

// Profile returns the currently loaded seccomp profile.
func (c *Config) Profile() *seccomp.Seccomp {
	return c.profile
}

// Setup can be used to setup the seccomp profile.
func (c *Config) Setup(
	ctx context.Context,
	sys *imagetypes.SystemContext,
	msgChan chan Notification,
	containerID, containerName string,
	sandboxAnnotations, imageAnnotations map[string]string,
	specGenerator *generate.Generator,
	profileField *types.SecurityProfile,
) (*Notifier, string, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Debugf(ctx, "Setup seccomp from profile field: %+v", profileField)

	// Specifically set profile fields always have a higher priority than OCI artifact annotations
	// TODO(sgrunert): allow merging OCI artifact profiles with security context ones.
	if profileField == nil || profileField.ProfileType == types.SecurityProfile_Unconfined {
		ociArtifactProfile, err := seccompociartifact.New().TryPull(ctx, sys, containerName, sandboxAnnotations, imageAnnotations)
		if err != nil {
			return nil, "", fmt.Errorf("try to pull OCI artifact seccomp profile: %w", err)
		}

		if ociArtifactProfile != nil {
			notifier, err := c.applyProfileFromBytes(ctx, ociArtifactProfile, msgChan, containerID, sandboxAnnotations, specGenerator)
			if err != nil {
				return nil, "", fmt.Errorf("apply profile from bytes: %w", err)
			}

			return notifier, "", nil
		}
	}

	// running w/o seccomp, aka unconfined
	if profileField == nil {
		specGenerator.Config.Linux.Seccomp = nil
		return nil, "", nil
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
		notifier, err := c.injectNotifier(ctx, msgChan, containerID, sandboxAnnotations, linuxSpecs)
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

	notifier, err := c.applyProfileFromBytes(ctx, file, msgChan, containerID, sandboxAnnotations, specGenerator)
	if err != nil {
		return nil, "", fmt.Errorf("apply profile from bytes: %w", err)
	}

	return notifier, localhostRef, nil
}

// Setup can be used to setup the seccomp profile.
func (c *Config) applyProfileFromBytes(
	ctx context.Context,
	fileBytes []byte,
	msgChan chan Notification,
	containerID string,
	sandboxAnnotations map[string]string,
	specGenerator *generate.Generator,
) (*Notifier, error) {
	linuxSpecs, err := seccomp.LoadProfileFromBytes(fileBytes, specGenerator.Config)
	if err != nil {
		return nil, fmt.Errorf("load local profile: %w", err)
	}

	notifier, err := c.injectNotifier(ctx, msgChan, containerID, sandboxAnnotations, linuxSpecs)
	if err != nil {
		return nil, fmt.Errorf("inject notifier: %w", err)
	}

	specGenerator.Config.Linux.Seccomp = linuxSpecs
	return notifier, nil
}
