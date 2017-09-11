package main

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/kubernetes-incubator/cri-o/libpod/images"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

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

	args := c.Args()
	var image string
	if len(args) == 1 {
		image = args[0]
	}
	if len(args) > 1 {
		return errors.New("too many arguments. Requires exactly 1")
	}

	runtime, err := getRuntime(c)
	if err != nil {
		return errors.Wrapf(err, "could not get runtime")
	}

	input := c.String("input")

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

	var output io.Writer
	if !c.Bool("quiet") {
		output = os.Stdout
	}
	src := images.DockerArchive + ":" + input
	if err := runtime.PullImage(src, false, output); err != nil {
		src = images.OCIArchive + ":" + input
		// generate full src name with specified image:tag
		if image != "" {
			src = src + ":" + image
		}
		if err := runtime.PullImage(src, false, output); err != nil {
			return errors.Wrapf(err, "error pulling %q", src)
		}
	}

	return nil
}
