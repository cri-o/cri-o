package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/containers/storage"
	libkpodimage "github.com/kubernetes-incubator/cri-o/libkpod/image"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	dockerArchive = "docker-archive:"
)

type saveOptions struct {
	output string
	quiet  bool
	images []string
}

var (
	saveFlags = []cli.Flag{
		cli.StringFlag{
			Name:  "output, o",
			Usage: "Write to a file, default is STDOUT",
			Value: "/dev/stdout",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "Suppress the output",
		},
	}
	saveDescription = "Save an image to docker-archive on the local machine"
	saveCommand     = cli.Command{
		Name:        "save",
		Usage:       "Save image to an archive",
		Description: saveDescription,
		Flags:       saveFlags,
		Action:      saveCmd,
		ArgsUsage:   "",
	}
)

// saveCmd saves the image to either docker-archive or oci
func saveCmd(c *cli.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return errors.Errorf("need at least 1 argument")
	}

	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "could not get config")
	}
	store, err := getStore(config)
	if err != nil {
		return err
	}

	output := c.String("output")
	quiet := c.Bool("quiet")

	if output == "/dev/stdout" {
		fi := os.Stdout
		if logrus.IsTerminal(fi) {
			return errors.Errorf("refusing to save to terminal. Use -o flag or redirect")
		}
	}

	opts := saveOptions{
		output: output,
		quiet:  quiet,
		images: args,
	}

	return saveImage(store, opts)
}

// saveImage pushes the image to docker-archive or oci by
// calling pushImage
func saveImage(store storage.Store, opts saveOptions) error {
	dst := dockerArchive + opts.output

	pushOpts := libkpodimage.CopyOptions{
		SignaturePolicyPath: "",
		Store:               store,
	}

	// only one image is supported for now
	// future pull requests will fix this
	for _, image := range opts.images {
		dest := dst + ":" + image
		if err := libkpodimage.PushImage(image, dest, pushOpts); err != nil {
			return errors.Wrapf(err, "unable to save %q", image)
		}
	}
	return nil
}
