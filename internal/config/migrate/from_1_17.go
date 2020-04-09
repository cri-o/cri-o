package migrate

import (
	"strings"

	"github.com/cri-o/cri-o/internal/config/apparmor"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/sirupsen/logrus"
)

// migrateFrom1_17 migrates a config from the 1.17.x version
func migrateFrom1_17(cfg *config.Config) error {
	// Remove NET_RAW and SYS_CHROOT capability by default
	// https://github.com/cri-o/cri-o/pull/3119
	newDefaultCapabilities := []string{}
	logrus.Infof("Checking for NET_RAW and SYS_CHROOT capabilities, which have been removed per default")
	for _, cap := range cfg.DefaultCapabilities {
		if cap == "NET_RAW" || cap == "SYS_CHROOT" {
			logrus.Infof(`Removing "default_capabilities" entry %q`, cap)
			continue
		}
		newDefaultCapabilities = append(newDefaultCapabilities, cap)
	}
	cfg.DefaultCapabilities = newDefaultCapabilities

	// Change AppArmor profile to not contain version info any more
	// https://github.com/cri-o/cri-o/pull/3287
	logrus.Infof("Checking for default AppArmor profile, which does not contain the version number any more")
	if cfg.ApparmorProfile != apparmor.DefaultProfile && strings.Contains(
		cfg.ApparmorProfile, apparmor.DefaultProfile,
	) {
		cfg.ApparmorProfile = apparmor.DefaultProfile
		logrus.Infof(`Changing "apparmor_profile" to %q`, cfg.ApparmorProfile)
	}

	// Changing the default error log level to info
	const newLogLevel = "info"
	logrus.Infof("Checking for the log level, which has changed from error to info")
	if cfg.LogLevel == "error" {
		cfg.LogLevel = newLogLevel
		logrus.Infof(`Changing "log_level" to %q`, newLogLevel)
	}

	// Change CtrStopTimeout to the new minimum value
	// https://github.com/cri-o/cri-o/pull/3282
	logrus.Infof("Checking for ctr_stop_timeout, which now has a minimum value of 30")
	const newCtrStopTimeout = 30
	if cfg.CtrStopTimeout < newCtrStopTimeout {
		cfg.CtrStopTimeout = newCtrStopTimeout
		logrus.Infof(`Changing "ctr_stop_timeout" to %d`, cfg.CtrStopTimeout)
	}

	return nil
}
