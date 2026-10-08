//go:build seccomp && linux && cgo

package seccomp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"go.podman.io/common/pkg/seccomp"
	imagetypes "go.podman.io/image/v5/types"
	json "github.com/json-iterator/go"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
	spmseccomp "sigs.k8s.io/security-profiles-merger/seccomp"

	"github.com/cri-o/cri-o/internal/config/seccomp/seccompociartifact"
	"github.com/cri-o/cri-o/internal/log"
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
		for _, remove := range removeSyscalls {
			validateSyscallIndex(prof, remove.Name, remove.ParentStructIndex, remove.Index)
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

		prof.Syscalls = append(prof.Syscalls,
			&seccomp.Syscall{
				Name:   "clone",
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
				Name:   "clone",
				Action: seccomp.ActErrno,
				Errno:  "EPERM",
				Args: []*seccomp.Arg{
					{Index: flagsIndex, Value: unix.CLONE_NEWNS, ValueTwo: unix.CLONE_NEWNS, Op: seccomp.OpMaskedEqual},
					{Index: flagsIndex, Value: unix.CLONE_NEWUTS, ValueTwo: unix.CLONE_NEWUTS, Op: seccomp.OpMaskedEqual},
					{Index: flagsIndex, Value: unix.CLONE_NEWIPC, ValueTwo: unix.CLONE_NEWIPC, Op: seccomp.OpMaskedEqual},
					{Index: flagsIndex, Value: unix.CLONE_NEWUSER, ValueTwo: unix.CLONE_NEWUSER, Op: seccomp.OpMaskedEqual},
					{Index: flagsIndex, Value: unix.CLONE_NEWPID, ValueTwo: unix.CLONE_NEWPID, Op: seccomp.OpMaskedEqual},
					{Index: flagsIndex, Value: unix.CLONE_NEWNET, ValueTwo: unix.CLONE_NEWNET, Op: seccomp.OpMaskedEqual},
					{Index: flagsIndex, Value: unix.CLONE_NEWCGROUP, ValueTwo: unix.CLONE_NEWCGROUP, Op: seccomp.OpMaskedEqual},
				},
				Excludes: seccomp.Filter{
					Caps: []string{"CAP_SYS_ADMIN"},
				},
			},
			// Because seccomp currently can't compare the data inside struct and the flags in clone3 are hidden in a struct,
			// seccomp can't block clone3 based on its flags. To force it to use only clone, we make clone3 return ENOSYS,
			// so that glibc can fall back to clone in the same way as https://github.com/moby/moby/pull/42681.
			&seccomp.Syscall{
				Name:   "clone3",
				Action: seccomp.ActErrno,
				Errno:  "ENOSYS",
				Excludes: seccomp.Filter{
					Caps: []string{"CAP_SYS_ADMIN"},
				},
			})
		defaultProfile = prof
	})

	return defaultProfile
}

// validateSyscallIndex checks if the syscall's index matches the default profile's index.
// We know the default profile at compile time, though a vendor change may update it.
// Panic on error and have CI catch errors on vendor bumps to avoid combing through.
func validateSyscallIndex(prof *seccomp.Seccomp, name string, parentStructIndex, index int) {
	if prof.Syscalls[parentStructIndex].Names[index] == name {
		return
	}

	var msg string
	i := slices.Index(prof.Syscalls[parentStructIndex].Names, name)
	if i == -1 {
		msg = fmt.Sprintf("Change the ParentStructIndex for %q", name)
	} else {
		msg = fmt.Sprintf("Change the Index for %q to %d", name, i)
	}
	logrus.Fatalf(
		`The default internal seccomp policy has been changed, and CRI-O can't adjust some risky syscalls.
You are likely seeing this error because "go.podman.io/common/pkg/seccomp" was updated.
Please contact the developers or change "DefaultProfile()" in "internal/config/seccomp/seccomp.go"
to match the updated policy as per the following hint: %s`, msg,
	)
}

func removeStringFromSlice(s []string, i int) []string {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

// Config is the global seccomp configuration type.
type Config struct {
	enabled      bool
	notifierPath string

	// state holds the profiles, which reloads replace while containers are
	// created.
	state atomic.Pointer[profileState]
}

// profileState holds the loaded profile and the baseline derived from it.
type profileState struct {
	profile *seccomp.Seccomp

	// baseline is the floor OCI artifact profiles are intersected with.
	baseline *baselineState
}

// baselineState describes the floor of OCI artifact profiles.
type baselineState struct {
	// configured is the baseline profile of the configuration, nil if none
	// is configured and the floor is the loaded profile.
	configured *seccomp.Seccomp

	// withProfile adds the loaded profile to a configured baseline, for a
	// runtime handler with a seccomp profile of its own.
	withProfile bool

	// err is why the floor cannot be merged, which makes every OCI profile
	// fail.
	err error
}

// New creates a new default seccomp configuration instance.
func New() *Config {
	c := &Config{
		enabled:      seccomp.IsEnabled(),
		notifierPath: "/var/run/crio/seccomp",
	}
	c.state.Store(&profileState{profile: DefaultProfile()})

	return c
}

// Replace replaces the profiles with the ones of another configuration, at
// once.
func (c *Config) Replace(other *Config) {
	c.state.Store(other.state.Load())
}

// setProfile replaces the loaded profile, which invalidates the baseline
// derived from it until it is loaded again.
func (c *Config) setProfile(profile *seccomp.Seccomp) {
	c.state.Store(&profileState{profile: profile})
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

	profile, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("open seccomp profile: %w", err)
	}

	tmpProfile := &seccomp.Seccomp{}
	if err := json.Unmarshal(profile, tmpProfile); err != nil {
		return fmt.Errorf("decoding seccomp profile failed: %w", err)
	}

	c.setProfile(tmpProfile)
	logrus.Infof("Successfully loaded seccomp profile %q", profilePath)
	logrus.Tracef("Current seccomp profile content: %s", profile)
	return nil
}

// LoadDefaultProfile sets the internal default profile.
func (c *Config) LoadDefaultProfile() error {
	logrus.Info("Using the internal default seccomp profile")
	c.setProfile(DefaultProfile())

	if logrus.IsLevelEnabled(logrus.TraceLevel) {
		profileString, err := json.MarshalToString(DefaultProfile())
		if err != nil {
			return fmt.Errorf("marshal default seccomp profile to string: %w", err)
		}
		logrus.Tracef("Default seccomp profile content: %s", profileString)
	}

	return nil
}

// LoadBaselineProfile loads the profile from the provided path as the baseline
// that OCI artifact profiles are intersected with. An empty path selects the
// loaded profile, which RuntimeDefault stands for, so it has to be called
// after loading that one. withProfile makes the floor the intersection of the
// baseline and the loaded profile, for a runtime handler with a profile of its
// own. The baseline is only replaced if loading succeeds. This method will not
// fail if seccomp is disabled.
func (c *Config) LoadBaselineProfile(profilePath string, withProfile bool) error {
	if c.IsDisabled() {
		return nil
	}

	state := &baselineState{withProfile: withProfile}

	if profilePath != "" {
		data, err := os.ReadFile(profilePath)
		if err != nil {
			return fmt.Errorf("open seccomp baseline profile: %w", err)
		}

		profile := &seccomp.Seccomp{}
		if err := json.Unmarshal(data, profile); err != nil {
			return fmt.Errorf("decoding seccomp baseline profile failed: %w", err)
		}

		if err := validateBaseline(profile, false); err != nil {
			return fmt.Errorf("validate seccomp baseline profile %q: %w", profilePath, err)
		}

		state.configured = profile

		logrus.Infof("Successfully loaded seccomp baseline profile %q", profilePath)
	}

	// Refusing a profile that used to load would break upgrades, so only
	// OCI profiles become unsupported with it.
	if state.configured == nil || withProfile {
		if err := validateBaseline(c.Profile(), true); err != nil {
			logrus.Warnf("The seccomp profile is not a valid baseline, OCI artifact profiles are unsupported: %v", err)

			state.err = fmt.Errorf("invalid seccomp profile as baseline: %w", err)
		}
	}

	c.state.Store(&profileState{profile: c.Profile(), baseline: state})

	return nil
}

// OCIProfilesSupported returns true if OCI artifact profiles can be merged
// with the baseline.
func (c *Config) OCIProfilesSupported() bool {
	state := c.state.Load().baseline

	return !c.IsDisabled() && state != nil && state.err == nil
}

// validateBaseline checks that a profile can be merged as the baseline.
// Rendering it without a container spec keeps every capability filtered entry.
// A profile without a filter restricts nothing, which only suits a runtime
// default.
func validateBaseline(profile *seccomp.Seccomp, unconfinedOK bool) error {
	linuxSpecs, err := seccomp.LoadProfileFromConfig(profile, nil)
	if err != nil {
		return err
	}

	if linuxSpecs == nil {
		if unconfinedOK {
			return nil
		}

		return errors.New("profile defines neither a default action nor syscalls")
	}

	return spmseccomp.Validate(linuxSpecs)
}

// renderProfile converts a node-local profile into the form of the runtime
// spec for the capabilities and the architecture of the container. It returns
// nil for a profile without a filter, which runs containers unconfined: such
// a profile restricts nothing, so it stays out of the merge.
func renderProfile(profile *seccomp.Seccomp, spec *rspec.Spec) (*rspec.LinuxSeccomp, error) {
	return seccomp.LoadProfileFromConfig(profile, spec)
}

// BaselineProfile returns the configured baseline profile, or the loaded
// profile if none is configured.
func (c *Config) BaselineProfile() *seccomp.Seccomp {
	if state := c.state.Load().baseline; state != nil && state.configured != nil {
		return state.configured
	}

	return c.Profile()
}

// IsDisabled returns true if seccomp is disabled either via the missing
// `seccomp` buildtag or globally by the system.
func (c *Config) IsDisabled() bool {
	return !c.enabled
}

// Profile returns the currently loaded seccomp profile.
func (c *Config) Profile() *seccomp.Seccomp {
	return c.state.Load().profile
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
	graphRoot string,
	ociProfiles OCIProfiles,
) (*Notifier, string, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	log.Debugf(ctx, "Setup seccomp from profile field: %+v", profileField)

	// Specifically set profile fields always have a higher priority than OCI artifact annotations
	// TODO(sgrunert): allow merging OCI artifact profiles with security context ones.
	if profileField == nil || profileField.ProfileType == types.SecurityProfile_Unconfined {
		store, err := seccompociartifact.New(graphRoot, sys)
		if err != nil {
			return nil, "", fmt.Errorf("create OCI artifact seccomp profile store: %w", err)
		}
		ociArtifactProfile, err := store.TryPull(ctx, containerName, sandboxAnnotations, imageAnnotations)
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

	if profileField.GetProfileType() == types.SecurityProfile_OCI {
		linuxSpecs, err := c.mergeOCIProfile(ctx, ociProfiles, profileField, specGenerator.Config)
		if err != nil {
			return nil, "", fmt.Errorf("merge OCI profile: %w", err)
		}

		notifier, err := c.injectNotifier(ctx, msgChan, containerID, sandboxAnnotations, linuxSpecs)
		if err != nil {
			return nil, "", fmt.Errorf("inject notifier: %w", err)
		}

		specGenerator.Config.Linux.Seccomp = linuxSpecs

		return notifier, profileField.GetOciRef(), nil
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

// mergeOCIProfile returns the profile for the OCI profile type: the pulled
// profile intersected with the baseline and the optional base profile.
func (c *Config) mergeOCIProfile(
	ctx context.Context,
	ociProfiles OCIProfiles,
	profileField *types.SecurityProfile,
	spec *rspec.Spec,
) (*rspec.LinuxSeccomp, error) {
	if ociProfiles == nil {
		return nil, errors.New("OCI seccomp profiles are not supported")
	}

	profiles := c.state.Load()

	state := profiles.baseline
	if state == nil {
		return nil, errors.New("seccomp baseline profile not loaded")
	}

	if state.err != nil {
		return nil, state.err
	}

	// The floor is the configured baseline, the loaded profile, or both.
	floor := []*seccomp.Seccomp{profiles.profile}
	if state.configured != nil {
		floor = []*seccomp.Seccomp{state.configured}
		if state.withProfile {
			floor = append(floor, profiles.profile)
		}
	}

	baseline, err := intersectLocal(floor, spec)
	if err != nil {
		return nil, fmt.Errorf("load baseline profile: %w", err)
	}

	var base *rspec.LinuxSeccomp

	// The zero value of the type selects RuntimeDefault, so only a base
	// profile that is not set means the configured baseline alone.
	baseProfile := profileField.GetBaseProfile()

	switch {
	case baseProfile == nil:

	case baseProfile.GetType() == types.SecurityProfileBase_RuntimeDefault:
		// Unless the floor includes it already.
		if state.configured != nil && !state.withProfile {
			base, err = renderProfile(profiles.profile, spec)
			if err != nil {
				return nil, fmt.Errorf("load default profile: %w", err)
			}
		}

	case baseProfile.GetType() == types.SecurityProfileBase_Localhost:
		localhostRef := filepath.FromSlash(baseProfile.GetLocalhostRef())

		file, err := os.ReadFile(localhostRef)
		if err != nil {
			return nil, fmt.Errorf("unable to load base profile %q: %w", localhostRef, err)
		}

		base, err = seccomp.LoadProfileFromBytes(file, spec)
		if err != nil {
			return nil, fmt.Errorf("load base profile %q: %w", localhostRef, err)
		}

	default:
		return nil, fmt.Errorf("unsupported base profile type %s", baseProfile.GetType())
	}

	return ociProfiles.MergeSeccomp(ctx, profileField.GetOciRef(), baseline, base)
}

// intersectLocal renders the node-local profiles and intersects them. It
// returns nil if none of them restricts anything.
func intersectLocal(profiles []*seccomp.Seccomp, spec *rspec.Spec) (*rspec.LinuxSeccomp, error) {
	var rendered []*rspec.LinuxSeccomp

	for _, profile := range profiles {
		linuxSpecs, err := renderProfile(profile, spec)
		if err != nil {
			return nil, err
		}

		if linuxSpecs != nil {
			rendered = append(rendered, linuxSpecs)
		}
	}

	switch len(rendered) {
	case 0:
		return nil, nil

	case 1:
		return rendered[0], nil

	default:
		return spmseccomp.Intersect(rendered...)
	}
}
