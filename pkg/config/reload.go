package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/containers/image/v5/pkg/sysregistriesv2"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/sirupsen/logrus"
	"tags.cncf.io/container-device-interface/pkg/cdi"
)

// Reload reloads the configuration for the single crio.conf and the drop-in
// configuration directory.
func (c *Config) Reload() error {
	logrus.Infof("Reloading configuration")

	// Reload the config
	newConfig, err := DefaultConfig()
	if err != nil {
		return fmt.Errorf("unable to create default config")
	}

	if _, err := os.Stat(c.singleConfigPath); !os.IsNotExist(err) {
		logrus.Infof("Updating config from file %s", c.singleConfigPath)
		if err := newConfig.UpdateFromFile(c.singleConfigPath); err != nil {
			return err
		}
	} else {
		logrus.Infof("Skipping not-existing config file %q", c.singleConfigPath)
	}

	if _, err := os.Stat(c.dropInConfigDir); !os.IsNotExist(err) {
		logrus.Infof("Updating config from path %s", c.dropInConfigDir)
		if err := newConfig.UpdateFromPath(c.dropInConfigDir); err != nil {
			return err
		}
	} else {
		logrus.Infof("Skipping not-existing config path %q", c.dropInConfigDir)
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
	cdi.GetRegistry(cdi.WithSpecDirs(newConfig.CDISpecDirs...))

	return nil
}

// logConfig logs a config set operation as with info verbosity. Please always
// use this function for setting configuration options to ensure consistent
// log outputs
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

// ReloadPinnedImages updates the PinnedImages with the provided `newConfig`.
func (c *Config) ReloadPinnedImages(newConfig *Config) {
	updatedPinnedImages := make([]string, len(newConfig.PinnedImages))
	for i, image := range newConfig.PinnedImages {
		if i < len(c.PinnedImages) && image == c.PinnedImages[i] {
			updatedPinnedImages[i] = c.PinnedImages[i]
		} else {
			updatedPinnedImages[i] = image
		}
	}
	logrus.Infof("Updated new pinned images: %+v", updatedPinnedImages)
	c.PinnedImages = updatedPinnedImages
}

// ReloadRegistries reloads the registry configuration from the Configs
// `SystemContext`. The method errors in case of any update failure.
func (c *Config) ReloadRegistries() error {
	registries, err := sysregistriesv2.TryUpdatingCache(c.SystemContext)
	if err != nil {
		return fmt.Errorf(
			"system registries reload failed: %s: %w",
			sysregistriesv2.ConfigPath(c.SystemContext),
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
	if err := c.seccompConfig.LoadProfile(newConfig.SeccompProfile); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("unable to load seccomp profile: %w", err)
		}

		logrus.Info("Specified profile does not exist on disk")
		if err := c.seccompConfig.LoadDefaultProfile(); err != nil {
			return fmt.Errorf("load default seccomp profile: %w", err)
		}
	}

	c.SeccompProfile = newConfig.SeccompProfile
	logConfig("seccomp_profile", c.SeccompProfile)
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

// ReloadBlockIOConfig reloads the blockio configuration from the new config
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

// ReloadRdtConfig reloads the RDT configuration if changed
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

// ReloadRuntimes reloads the runtimes configuration if changed
func (c *Config) ReloadRuntimes(newConfig *Config) error {
	var updated bool
	if !RuntimesEqual(c.Runtimes, newConfig.Runtimes) {
		logrus.Infof("Updating runtime configuration")
		c.Runtimes = newConfig.Runtimes
		updated = true
	}

	if c.DefaultRuntime != newConfig.DefaultRuntime {
		c.DefaultRuntime = newConfig.DefaultRuntime
		if err := c.ValidateDefaultRuntime(); err != nil {
			return fmt.Errorf("unable to reload runtimes: %w", err)
		}
		logConfig("default_runtime", c.DefaultRuntime)
		updated = true
	}

	if !updated {
		return nil
	}

	if err := c.ValidateRuntimes(); err != nil {
		return fmt.Errorf("unabled to reload runtimes: %w", err)
	}

	return nil
}
