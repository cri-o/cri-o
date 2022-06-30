package main

import (
	"errors"
	"fmt"
	"os"

	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/criocli"
	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/internal/version"
	crioconf "github.com/cri-o/cri-o/pkg/config"
	json "github.com/json-iterator/go"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var wipeCommand = &cli.Command{
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
	config, err := criocli.GetConfigFromContext(c)
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
	if shouldWipeContainers && shutdownWasUnclean(config) {
		return handleCleanShutdown(config, store)
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

	cstore := ContainerStore{store}
	if err := cstore.wipeCrio(shouldWipeImages); err != nil {
		return err
	}

	return nil
}

func shutdownWasUnclean(config *crioconf.Config) bool {
	// CleanShutdownFile not configured, skip
	if config.CleanShutdownFile == "" {
		return false
	}
	// CleanShutdownFile isn't supported, skip
	if _, err := os.Stat(config.CleanShutdownSupportedFileName()); err != nil {
		return false
	}
	// CleanShutdownFile is present, indicating clean shutdown
	if _, err := os.Stat(config.CleanShutdownFile); err == nil {
		return false
	}
	return true
}

func handleCleanShutdown(config *crioconf.Config, store cstorage.Store) error {
	logrus.Infof("File %s not found. Wiping storage directory %s because of suspected dirty shutdown", config.CleanShutdownFile, store.GraphRoot())
	// If we do not do this, we may leak other resources that are not directly in the graphroot.
	// Erroring here should not be fatal though, it's a best effort cleanup
	if err := store.Wipe(); err != nil {
		logrus.Infof("Failed to wipe storage cleanly: %v", err)
	}
	// unmount storage or else we will fail with EBUSY
	if _, err := store.Shutdown(false); err != nil {
		return fmt.Errorf("failed to shutdown storage before wiping: %w", err)
	}
	// totally remove storage, whatever is left (possibly orphaned layers)
	if err := os.RemoveAll(store.GraphRoot()); err != nil {
		return fmt.Errorf("failed to remove storage directory: %w", err)
	}
	return nil
}

type ContainerStore struct {
	store cstorage.Store
}

func (c ContainerStore) wipeCrio(shouldWipeImages bool) error {
	crioContainers, crioImages, err := c.getCrioContainersAndImages()
	if err != nil {
		return err
	}
	if len(crioContainers) != 0 {
		logrus.Infof("Wiping containers")
	}
	for _, id := range crioContainers {
		c.deleteContainer(id)
	}
	if shouldWipeImages {
		if len(crioImages) != 0 {
			logrus.Infof("Wiping images")
		}
		for _, id := range crioImages {
			c.deleteImage(id)
		}
	}
	return nil
}

func (c ContainerStore) getCrioContainersAndImages() (crioContainers, crioImages []string, _ error) {
	containers, err := c.store.Containers()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return crioContainers, crioImages, err
		}
		logrus.Errorf("Could not read containers and sandboxes: %v", err)
	}

	for i := range containers {
		id := containers[i].ID
		metadataString, err := c.store.Metadata(id)
		if err != nil {
			continue
		}

		metadata := storage.RuntimeContainerMetadata{}
		if err := json.Unmarshal([]byte(metadataString), &metadata); err != nil {
			continue
		}
		if !storage.IsCrioContainer(&metadata) {
			continue
		}
		crioContainers = append(crioContainers, id)
		crioImages = append(crioImages, containers[i].ImageID)
	}
	return crioContainers, crioImages, nil
}

func (c ContainerStore) deleteContainer(id string) {
	if mounted, err := c.store.Unmount(id, true); err != nil || mounted {
		logrus.Errorf("Unable to unmount container %s: %v", id, err)
		return
	}
	if err := c.store.DeleteContainer(id); err != nil {
		logrus.Errorf("Unable to delete container %s: %v", id, err)
		return
	}
	logrus.Infof("Deleted container %s", id)
}

func (c ContainerStore) deleteImage(id string) {
	if _, err := c.store.DeleteImage(id, true); err != nil {
		logrus.Errorf("Unable to delete image %s: %v", id, err)
		return
	}
	logrus.Infof("Deleted image %s", id)
}
