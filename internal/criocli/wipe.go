package criocli

import (
	"fmt"
	"os"

	cstorage "github.com/containers/storage"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/version"
)

var WipeCommand = &cli.Command{
	Name:   "wipe",
	Usage:  "wipe CRI-O's container and image storage",
	Action: crioWipe,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "force wipe by skipping the version check",
		},
	},
}

func crioWipe(c *cli.Context) error {
	config, err := GetConfigFromContext(c)
	if err != nil {
		return err
	}

	store, err := config.GetStore()
	if err != nil {
		return err
	}

	shouldWipeImages := true
	shouldWipeContainers := true

	if !c.IsSet("force") {
		// First, check if the node was rebooted.
		// We know this happened because the VersionFile (which lives in a tmpfs)
		// will not be there.
		shouldWipeContainers, err = version.ShouldCrioWipe(config.VersionFile)
		if err != nil {
			logrus.Infof("Checking whether cri-o should wipe containers: %v", err)
		}

		// there are two locations we check before wiping:
		// one in a temporary directory. This is to check whether the node has rebooted.
		// if so, we should remove containers
		// another is needed in a persistent directory. This is to check whether we've upgraded
		// if we've upgraded, we should wipe images
		shouldWipeImages, err = version.ShouldCrioWipe(config.VersionFilePersist)
		if err != nil {
			logrus.Infof("%v: triggering wipe of images", err.Error())
		}
	}

	// Then, check whether crio has shutdown with time to sync.
	// Note: this is only needed if the node rebooted.
	// If there wasn't time to sync, we should clear the storage directory
	if shouldWipeContainers && lib.ShutdownWasUnclean(config) {
		logrus.Infof(
			"File %s not found. Wiping storage directory %s because of suspected unclean shutdown",
			config.CleanShutdownFile,
			store.GraphRoot(),
		)

		wipeMarkerFile := "/run/crio/crio-wipe-done"
		if _, err := os.Stat(wipeMarkerFile); err == nil {
			logrus.Infof("Unclean shutdown check already succeeded by previous crio wipe command")

			return nil
		}

		// This will fail if there are any containers currently running.
		if err := lib.RemoveStorageDirectory(config, store, false); err != nil {
			return fmt.Errorf("failed to remove storage directory %w", err)
		}

		if err = os.WriteFile(wipeMarkerFile, []byte("done"), 0o644); err != nil {
			logrus.Warnf("Failed to create crio wipe marker file: %v", err)
		}
	}

	// If crio is configured to wipe internally (and `--force` wasn't set)
	// the `crio wipe` command has nothing left to do,
	// as the remaining work will be done on server startup.
	if config.InternalWipe && !c.IsSet("force") {
		return nil
	}

	logrus.Infof("Internal wipe not set, meaning crio wipe will wipe. In the future, all wipes after reboot will happen when starting the crio server.")

	// if we should not wipe, exit with no error
	if !shouldWipeContainers {
		// we should not wipe images without wiping containers
		// in a future release, we should wipe both container and images if only shouldWipeImages is true.
		// However, now, we cannot expect users to have version-file-persist after having upgraded
		// to this version. Skip the wipe, for now, and log about it.
		if shouldWipeImages {
			logrus.Infof("Legacy version-file path found, but new version-file-persist path not. Skipping wipe")
		}

		logrus.Infof("Version unchanged and node not rebooted; no wipe needed")

		return nil
	}

	return wipeCrio(store, shouldWipeImages && !config.NeverWipeImages)
}

func wipeCrio(store cstorage.Store, shouldWipeImages bool) error {
	crioContainers, crioImages, err := lib.GetCrioContainersAndImages(store)
	if err != nil {
		return err
	}

	if len(crioContainers) != 0 {
		logrus.Infof("Wiping containers")
	}

	for _, id := range crioContainers {
		lib.DeleteContainer(store, id)
	}

	if shouldWipeImages {
		if len(crioImages) != 0 {
			logrus.Infof("Wiping images")
		}

		for _, id := range crioImages {
			lib.DeleteImage(store, id)
		}
	} else if !shouldWipeImages && len(crioImages) != 0 {
		logrus.Infof("Skipping image wipe due to never_wipe_images setting")
	}

	return nil
}
