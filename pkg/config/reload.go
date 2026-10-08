package config

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sirupsen/logrus"
	"go.podman.io/image/v5/pkg/sysregistriesv2"
	"tags.cncf.io/container-device-interface/pkg/cdi"

	"github.com/cri-o/cri-o/internal/log"
)

// Reload reloads the configuration for the single crio.conf and the drop-in
// configuration directory.
func (c *Config) Reload(ctx context.Context) error {
	log.Infof(ctx, "Reloading configuration")

	// Reload the config
	newConfig, err := DefaultConfig()
	if err != nil {
		return fmt.Errorf("unable to create default config: %w", err)
	}

	if _, err := os.Stat(c.singleConfigPath); !os.IsNotExist(err) {
		if err := newConfig.UpdateFromFile(ctx, c.singleConfigPath); err != nil {
			return fmt.Errorf("update config from single file: %w", err)
		}
	} else {
		log.Infof(ctx, "Skipping not-existing config file %q", c.singleConfigPath)
	}

	if _, err := os.Stat(c.dropInConfigDir); !os.IsNotExist(err) {
		if err := newConfig.UpdateFromPath(ctx, c.dropInConfigDir); err != nil {
			return fmt.Errorf("update config from path: %w", err)
		}
	} else {
		log.Infof(ctx, "Skipping not-existing config path %q", c.dropInConfigDir)
	}

	// Reload all available options
	if err := c.ReloadLogLevel(newConfig); err != nil {
		return err
	}

	if err := c.ReloadLogFilter(newConfig); err != nil {
		return err
	}

	if err := c.ReloadPauseImage(newConfig); err != nil {
		return err
	}

	c.ReloadPinnedImages(newConfig)

	if err := c.ReloadRegistries(); err != nil {
		return err
	}

	c.ReloadDecryptionKeyConfig(newConfig)

	if err := c.ReloadSeccompProfile(newConfig); err != nil {
		return err
	}

	if err := c.ReloadAppArmorProfile(newConfig); err != nil {
		return err
	}

	if err := c.ReloadBlockIOConfig(newConfig); err != nil {
		return err
	}

	if err := c.ReloadRdtConfig(newConfig); err != nil {
		return err
	}

	if err := c.ReloadRuntimes(newConfig); err != nil {
		return err
	}

	if err := cdi.Configure(cdi.WithSpecDirs(newConfig.CDISpecDirs...)); err != nil {
		return err
	}

	return nil
}

// logConfig logs a config set operation as with info verbosity. Please always
// use this function for setting configuration options to ensure consistent
// log outputs.
func logConfig(option, value string) {
	logrus.Infof("Set config %s to %q", option, value)
}

// ReloadLogLevel updates the LogLevel with the provided `newConfig`. It errors
// if the level is not parsable.
func (c *Config) ReloadLogLevel(newConfig *Config) error {
	if c.LogLevel != newConfig.LogLevel {
		level, err := logrus.ParseLevel(newConfig.LogLevel)
		if err != nil {
			return err
		}
		// Always log this message without considering the current
		logrus.SetLevel(logrus.InfoLevel)
		logConfig("log_level", newConfig.LogLevel)

		logrus.SetLevel(level)

		c.LogLevel = newConfig.LogLevel
	}

	return nil
}

// ReloadLogFilter updates the LogFilter with the provided `newConfig`. It errors
// if the filter is not applicable.
func (c *Config) ReloadLogFilter(newConfig *Config) error {
	if c.LogFilter != newConfig.LogFilter {
		hook, err := log.NewFilterHook(newConfig.LogFilter)
		if err != nil {
			return err
		}

		logger := logrus.StandardLogger()
		log.RemoveHook(logger, "FilterHook")
		logConfig("log_filter", newConfig.LogFilter)
		logger.AddHook(hook)

		c.LogFilter = newConfig.LogFilter
	}

	return nil
}

func (c *Config) ReloadPauseImage(newConfig *Config) error {
	if c.PauseImage != newConfig.PauseImage {
		if _, err := newConfig.ParsePauseImage(); err != nil {
			return err
		}

		c.PauseImage = newConfig.PauseImage
		logConfig("pause_image", c.PauseImage)
	}

	if c.PauseImageAuthFile != newConfig.PauseImageAuthFile {
		if newConfig.PauseImageAuthFile != "" {
			if _, err := os.Stat(newConfig.PauseImageAuthFile); err != nil {
				return err
			}
		}

		c.PauseImageAuthFile = newConfig.PauseImageAuthFile
		logConfig("pause_image_auth_file", c.PauseImageAuthFile)
	}

	if c.PauseCommand != newConfig.PauseCommand {
		c.PauseCommand = newConfig.PauseCommand
		logConfig("pause_command", c.PauseCommand)
	}

	return nil
}

// ReloadPinnedImages replace the PinnedImages
// with the provided `newConfig.PinnedImages`.
// The method skips empty items and prints a log message.
func (c *Config) ReloadPinnedImages(newConfig *Config) {
	if len(newConfig.PinnedImages) == 0 {
		c.PinnedImages = []string{}

		logConfig("pinned_images", "[]")

		return
	}

	if cmp.Equal(c.PinnedImages, newConfig.PinnedImages,
		cmpopts.SortSlices(func(a, b string) bool {
			return a < b
		}),
	) {
		return
	}

	pinnedImages := []string{}

	for _, img := range newConfig.PinnedImages {
		if img != "" {
			pinnedImages = append(pinnedImages, img)
		}
	}

	logConfig("pinned_images", strings.Join(pinnedImages, ","))

	c.PinnedImages = pinnedImages
}

// ReloadRegistries reloads the registry configuration from the Configs
// `SystemContext`. The method errors in case of any update failure.
func (c *Config) ReloadRegistries() error {
	registries, err := sysregistriesv2.TryUpdatingCache(c.SystemContext)
	if err != nil {
		return fmt.Errorf(
			"system registries reload failed: %s: %w",
			sysregistriesv2.ConfigurationSourceDescription(c.SystemContext),
			err,
		)
	}

	logrus.Infof("Applied new registry configuration: %+v", registries)

	return nil
}

// ReloadDecryptionKeyConfig updates the DecryptionKeysPath with the provided
// `newConfig`.
func (c *Config) ReloadDecryptionKeyConfig(newConfig *Config) {
	if c.DecryptionKeysPath != newConfig.DecryptionKeysPath {
		logConfig("decryption_keys_path", newConfig.DecryptionKeysPath)
		c.DecryptionKeysPath = newConfig.DecryptionKeysPath
	}
}

// ReloadSeccompProfile reloads the seccomp profile from the new config if
// their paths differ.
func (c *Config) ReloadSeccompProfile(newConfig *Config) error {
	// Reload the seccomp profile in any case because its content could have
	// changed as well
	if newConfig.SeccompProfile == "" {
		if err := c.seccompConfig.LoadDefaultProfile(); err != nil {
			return fmt.Errorf("unable to load default seccomp profile: %w", err)
		}
	} else if err := c.seccompConfig.LoadProfile(newConfig.SeccompProfile); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("unable to load seccomp profile: %w", err)
		}

		logrus.Info("Seccomp profile does not exist on disk, fallback to internal default profile")

		if err := c.seccompConfig.LoadDefaultProfile(); err != nil {
			return fmt.Errorf("unable to load default seccomp profile: %w", err)
		}
	}

	c.SeccompProfile = newConfig.SeccompProfile
	logConfig("seccomp_profile", c.SeccompProfile)

	c.PrivilegedSeccompProfile = newConfig.PrivilegedSeccompProfile
	logConfig("privileged_seccomp_profile", c.PrivilegedSeccompProfile)

	return nil
}

// ReloadAppArmorProfile reloads the AppArmor profile from the new config if
// they differ.
func (c *Config) ReloadAppArmorProfile(newConfig *Config) error {
	if c.ApparmorProfile != newConfig.ApparmorProfile {
		if err := c.AppArmor().LoadProfile(newConfig.ApparmorProfile); err != nil {
			return fmt.Errorf("unable to reload apparmor_profile: %w", err)
		}

		c.ApparmorProfile = newConfig.ApparmorProfile
		logConfig("apparmor_profile", c.ApparmorProfile)
	}

	return nil
}

// ReloadBlockIOConfig reloads the blockio configuration from the new config.
func (c *Config) ReloadBlockIOConfig(newConfig *Config) error {
	if c.BlockIOConfigFile != newConfig.BlockIOConfigFile {
		if err := c.BlockIO().Load(newConfig.BlockIOConfigFile); err != nil {
			return fmt.Errorf("unable to reload blockio_config_file: %w", err)
		}

		c.BlockIOConfigFile = newConfig.BlockIOConfigFile
		logConfig("blockio_config_file", c.BlockIOConfigFile)
	}

	if c.BlockIOReload != newConfig.BlockIOReload {
		c.BlockIOReload = newConfig.BlockIOReload
		logConfig("blockio_reload", strconv.FormatBool(c.BlockIOReload))
	}

	return nil
}

// ReloadRdtConfig reloads the RDT configuration if changed.
func (c *Config) ReloadRdtConfig(newConfig *Config) error {
	if c.RdtConfigFile != newConfig.RdtConfigFile {
		if err := c.Rdt().Load(newConfig.RdtConfigFile); err != nil {
			return fmt.Errorf("unable to reload rdt_config_file: %w", err)
		}

		c.RdtConfigFile = newConfig.RdtConfigFile
		logConfig("rdt_config_file", c.RdtConfigFile)
	}

	return nil
}

// ReloadRuntimes reloads the runtimes configuration if changed.
func (c *Config) ReloadRuntimes(newConfig *Config) error {
	if RuntimesEqual(c.Runtimes, newConfig.Runtimes) &&
		c.DefaultRuntime == newConfig.DefaultRuntime {
		return nil
	}

	logrus.Infof("Updating runtime configuration")

	// Validate the candidate runtime configuration before making it visible
	// to any consumer: all validation and inheritance happens on the
	// candidate, while the shared configuration is only replaced once
	// everything succeeded. This way a failed reload cannot leave the
	// shared configuration in a half-applied state.
	if newConfig.cgroupManager == nil {
		newConfig.cgroupManager = c.cgroupManager
	}

	if err := newConfig.ValidateDefaultRuntime(); err != nil {
		return fmt.Errorf("unable to reload runtimes: %w", err)
	}

	if err := newConfig.ValidateRuntimes(); err != nil {
		return fmt.Errorf("unable to reload runtimes: %w", err)
	}

	for name := range newConfig.Runtimes {
		if newConfig.Runtimes[name].seccompConfig != nil {
			newConfig.Runtimes[name].seccompConfig.SetNotifierPath(
				filepath.Join(filepath.Dir(c.Listen), "seccomp"),
			)
		}
	}

	// Store the validated runtime configuration and publish it as a single
	// atomic snapshot, which is the only state observed by the runtime
	// consumers. Concurrent readers therefore never observe a runtime table
	// with a mismatching default runtime or any other intermediate state.
	defaultChanged := c.DefaultRuntime != newConfig.DefaultRuntime

	c.RuntimeConfig.Runtimes = newConfig.Runtimes
	c.RuntimeConfig.DefaultRuntime = newConfig.DefaultRuntime
	c.publishRuntimeSnapshot()

	if defaultChanged {
		logConfig("default_runtime", c.DefaultRuntime)
	}

	return nil
}
