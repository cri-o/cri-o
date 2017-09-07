package main

import (
	"io"
	"os"

	"io/ioutil"

	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/libpod/common"
	"github.com/kubernetes-incubator/cri-o/libpod/images"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type loadOptions struct {
	input string
	quiet bool
}

var (
	loadFlags = []cli.Flag{
		cli.StringFlag{
			Name:  "input, i",
			Usage: "Read from archive file, default is STDIN",
			Value: "/dev/stdin",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "Suppress the output",
		},
	}
	loadDescription = "Loads the image from docker-archive stored on the local machine."
	loadCommand     = cli.Command{
		Name:        "load",
		Usage:       "load an image from docker archive",
		Description: loadDescription,
		Flags:       loadFlags,
		Action:      loadCmd,
		ArgsUsage:   "",
	}
)

// loadCmd gets the image/file to be loaded from the command line
// and calls loadImage to load the image to containers-storage
func loadCmd(c *cli.Context) error {
	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "could not get config")
	}
	store, err := getStore(config)
	if err != nil {
		return err
	}

	args := c.Args()
	if len(args) > 0 {
		return errors.New("too many arguments. Requires exactly 1")
	}

	input := c.String("input")
	quiet := c.Bool("quiet")

	if input == "/dev/stdin" {
		fi, err := os.Stdin.Stat()
		if err != nil {
			return err
		}
		// checking if loading from pipe
		if !fi.Mode().IsRegular() {
			outFile, err := ioutil.TempFile("/var/tmp", "kpod")
			if err != nil {
				return errors.Errorf("error creating file %v", err)
			}
			defer outFile.Close()
			defer os.Remove(outFile.Name())

			inFile, err := os.OpenFile(input, 0, 0666)
			if err != nil {
				return errors.Errorf("error reading file %v", err)
			}
			defer inFile.Close()

			_, err = io.Copy(outFile, inFile)
			if err != nil {
				return errors.Errorf("error copying file %v", err)
			}

			input = outFile.Name()
		}
	}

	opts := loadOptions{
		input: input,
		quiet: quiet,
	}

	return loadImage(store, opts)
}

// loadImage loads the image from docker-archive or oci to containers-storage
// using the pullImage function
func loadImage(store storage.Store, opts loadOptions) error {
	systemContext := common.GetSystemContext("")

	src := dockerArchive + opts.input

	return images.PullImage(store, src, false, opts.quiet, systemContext)
}
