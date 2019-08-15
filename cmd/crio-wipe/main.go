package main

import (
	"encoding/json"
	"fmt"
	"os"

	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/pkg/clicommon"
	"github.com/cri-o/cri-o/internal/pkg/storage"
	"github.com/cri-o/cri-o/internal/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "crio-wipe"
	app.Usage = "A tool to clear CRI-O's container and image storage"
	app.Version = version.Version
	app.CommandNotFound = func(*cli.Context, string) { os.Exit(1) }
	app.OnUsageError = func(c *cli.Context, e error, b bool) error { return e }
	app.Action = crioWipe

	var err error
	app.Flags, app.Metadata, err = clicommon.GetFlagsAndMetadata()
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func crioWipe(c *cli.Context) error {
	_, config, err := clicommon.GetConfigFromContext(c)
	if err != nil {
		return err
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
		// TODO FIXME maybe have a better way to filter podman
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
