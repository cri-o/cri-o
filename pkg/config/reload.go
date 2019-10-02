package config

import (
	"fmt"
	"os"

	"github.com/cri-o/cri-o/internal/pkg/log"
	"github.com/sirupsen/logrus"
)

// Reload reloads the configuration with the config at the provided `fileName`
// path. The method errors in case of any read or update failure.
func (c *Config) Reload(fileName string) error {
	// Reload the config
	newConfig, err := DefaultConfig()
	if err != nil {
		return fmt.Errorf("unable to create default config")
	}
	if err := newConfig.UpdateFromFile(fileName); err != nil {
		return err
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

	return nil
}

// logConfig logs a config set operation as with info verbosity. Please always
// use this function for setting configuration options to ensure consistent
// log outputs
func logConfig(option, value string) {
	logrus.Infof("set config %s to %q", option, value)
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
