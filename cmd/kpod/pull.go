package main

import (
	"github.com/kubernetes-incubator/cri-o/libkpod/common"
	libkpodimage "github.com/kubernetes-incubator/cri-o/libkpod/image"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

var (
	pullFlags = []cli.Flag{
		cli.BoolFlag{
			// all-tags is hidden since it has not been implemented yet
			Name:   "all-tags, a",
			Hidden: true,
			Usage:  "Download all tagged images in the repository",
		},
	}

	pullDescription = "Pulls an image from a registry and stores it locally.\n" +
		"An image can be pulled using its tag or digest. If a tag is not\n" +
		"specified, the image with the 'latest' tag (if it exists) is pulled."
	pullCommand = cli.Command{
		Name:        "pull",
		Usage:       "pull an image from a registry",
		Description: pullDescription,
		Flags:       pullFlags,
		Action:      pullCmd,
		ArgsUsage:   "",
	}
)

// pullCmd gets the data from the command line and calls pullImage
// to copy an image from a registry to a local machine
func pullCmd(c *cli.Context) error {
	args := c.Args()
	if len(args) == 0 {
		logrus.Errorf("an image name must be specified")
		return nil
	}
	if len(args) > 1 {
		logrus.Errorf("too many arguments. Requires exactly 1")
		return nil
	}
	image := args[0]

	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "could not get config")
	}
	store, err := getStore(config)
	if err != nil {
		return err
	}

	allTags := c.Bool("all-tags")

	systemContext := common.GetSystemContext("")

	err = libkpodimage.PullImage(store, image, allTags, false, systemContext)
	if err != nil {
		return errors.Errorf("error pulling image from %q: %v", image, err)
	}
	return nil
}
