package migrate

import (
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/sirupsen/logrus"
)

// migrateFrom1_20 migrates a config from the 1.20.x version
func migrateFrom1_20(cfg *config.Config) error {
	// Upgrade pause image
	logrus.Infof("Checking for pause_image, which now should be k8s.gcr.io/pause:3.5 instead of k8s.gcr.io/pause:3.2")
	if cfg.PauseImage == "k8s.gcr.io/pause:3.2" {
		cfg.PauseImage = config.DefaultPauseImage
		logrus.Infof(`Changing "pause_image" to %s`, cfg.PauseImage)
	}

	return nil
}
