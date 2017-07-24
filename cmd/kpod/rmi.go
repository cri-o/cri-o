package main

import (
	"fmt"

	"github.com/containers/storage"
	libkpodimage "github.com/kubernetes-incubator/cri-o/libkpod/image"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var (
	rmiDescription = "removes one or more locally stored images."
	rmiFlags       = []cli.Flag{
		cli.BoolFlag{
			Name:  "force, f",
			Usage: "force removal of the image",
		},
	}
	rmiCommand = cli.Command{
		Name:        "rmi",
		Usage:       "removes one or more images from local storage",
		Description: rmiDescription,
		Action:      rmiCmd,
		ArgsUsage:   "IMAGE-NAME-OR-ID [...]",
		Flags:       rmiFlags,
	}
)

func rmiCmd(c *cli.Context) error {

	force := false
	if c.IsSet("force") {
		force = c.Bool("force")
	}

	args := c.Args()
	if len(args) == 0 {
		return errors.Errorf("image name or ID must be specified")
	}

	store, err := getStore(c)
	if err != nil {
		return err
	}

	for _, id := range args {
		image, err := libkpodimage.GetImage(store, id)
		if err != nil {
			return errors.Wrapf(err, "could not get image %q", id)
		}
		if image != nil {
			ctrIDs, err := runningContainers(image, store)
			if err != nil {
				return errors.Wrapf(err, "error getting running containers for image %q", id)
			}
			if len(ctrIDs) > 0 && len(image.Names) <= 1 {
				if force {
					removeContainers(ctrIDs, store)
				} else {
					for ctrID := range ctrIDs {
						return fmt.Errorf("Could not remove image %q (must force) - container %q is using its reference image", id, ctrID)
					}
				}
			}
			// If the user supplied an ID, we cannot delete the image if it is referred to by multiple tags
			if libkpodimage.MatchesID(image.ID, id) {
				if len(image.Names) > 1 && !force {
					return fmt.Errorf("unable to delete %s (must force) - image is referred to in multiple tags", image.ID)
				}
				// If it is forced, we have to untag the image so that it can be deleted
				image.Names = image.Names[:0]
			} else {
				name, err2 := libkpodimage.UntagImage(store, image, id)
				if err2 != nil {
					return err
				}
				fmt.Printf("untagged: %s", name)
			}

			if len(image.Names) > 0 {
				continue
			}
			id, err := libkpodimage.RemoveImage(image, store)
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", id)
		}
	}

	return nil
}

// Returns a list of running containers associated with the given ImageReference
// TODO: replace this with something in libkpod
func runningContainers(image *storage.Image, store storage.Store) ([]string, error) {
	ctrIDs := []string{}
	ctrStore, err := store.ContainerStore()
	if err != nil {
		return nil, err
	}

	containers, err := ctrStore.Containers()
	if err != nil {
		return nil, err
	}
	for _, ctr := range containers {
		if ctr.ImageID == image.ID {
			ctrIDs = append(ctrIDs, ctr.ID)
		}
	}
	return ctrIDs, nil
}

// TODO: replace this with something in libkpod
func removeContainers(ctrIDs []string, store storage.Store) error {
	ctrStore, err := store.ContainerStore()
	if err != nil {
		return err
	}
	for _, ctrID := range ctrIDs {
		if err = ctrStore.Delete(ctrID); err != nil {
			return errors.Wrapf(err, "could not remove container %q", ctrID)
		}
	}
	return nil
}
