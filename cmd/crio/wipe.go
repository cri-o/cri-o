package main

import (
	"encoding/json"
	"fmt"
	"os"

	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/pkg/criocli"
	"github.com/cri-o/cri-o/internal/pkg/storage"
	"github.com/cri-o/cri-o/internal/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var wipeCommand = cli.Command{
	Name:   "wipe",
	Usage:  "wipe CRI-O's container and image storage",
	Action: crioWipe,
}

func crioWipe(c *cli.Context) error {
	_, config, err := criocli.GetConfigFromContext(c)
	if err != nil {
		return err
	}

	// First, check if we need to upgrade at all
	shouldWipe, err := version.ShouldCrioWipe(config.VersionFile)
	if err != nil {
		fmt.Fprint(os.Stderr, err.Error())
	}

	// if we should not wipe, exit with no error
	if !shouldWipe {
		fmt.Println("major and minor version unchanged; no wipe needed")
		return nil
	}

	store, err := config.GetStore()
	if err != nil {
		return err
	}

	cstore := ContainerStore{store}
	if err := cstore.wipeCrio(); err != nil {
		return err
	}

	return nil
}

type ContainerStore struct {
	store cstorage.Store
}

func (c ContainerStore) wipeCrio() error {
	crioContainers, crioImages, err := c.getCrioContainersAndImages()
	if err != nil {
		return err
	}
	for _, id := range crioContainers {
		c.deleteContainer(id)
	}
	for _, id := range crioImages {
		c.deleteImage(id)
	}
	return nil
}

func (c ContainerStore) getCrioContainersAndImages() (crioContainers, crioImages []string, err error) {
	containers, err := c.store.Containers()
	if err != nil {
		if os.IsNotExist(errors.Cause(err)) {
			return crioContainers, crioImages, err
		}
		logrus.Warnf("could not read containers and sandboxes: %v", err)
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
		// CRI-O pods differ from libpod pods because they contain a PodName and PodID annotation
		if metadata.PodName == "" || metadata.PodID == "" {
			continue
		}
		crioContainers = append(crioContainers, id)
		crioImages = append(crioImages, containers[i].ImageID)
	}
	return crioContainers, crioImages, nil
}

func (c ContainerStore) deleteContainer(id string) {
	if mounted, err := c.store.Unmount(id, true); err != nil || mounted {
		logrus.Warnf("unable to unmount container %s: %v", id, err)
		return
	}
	if err := c.store.DeleteContainer(id); err != nil {
		logrus.Warnf("unable to delete container %s: %v", id, err)
		return
	}
	logrus.Infof("deleted container %s", id)
}

func (c ContainerStore) deleteImage(id string) {
	if _, err := c.store.DeleteImage(id, true); err != nil {
		logrus.Warnf("unable to delete image %s: %v", id, err)
		return
	}
	logrus.Infof("deleted image %s", id)
}
