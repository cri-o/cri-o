package main

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var runDescription = "Runs a command in a new container from the given image"

var runCommand = cli.Command{
	Name:        "run",
	Usage:       "run a command in a new container",
	Description: runDescription,
	Flags:       createFlags,
	Action:      runCmd,
	ArgsUsage:   "IMAGE [COMMAND [ARG...]]",
}

func runCmd(c *cli.Context) error {
	if len(c.Args()) != 1 {
		return errors.Errorf("must specify name of image to create from")
	}
	if err := validateFlags(c, createFlags); err != nil {
		return err
	}
	runtime, err := getRuntime(c)
	if err != nil {
		return errors.Wrapf(err, "error creating libpod runtime")
	}

	createConfig, err := parseCreateOpts(c)
	if err != nil {
		return err
	}

	runtimeSpec, err := createConfigToOCISpec(createConfig)
	if err != nil {
		return err
	}

	ctr, err := runtime.NewContainer(runtimeSpec)
	if err != nil {
		return err
	}

	// Should we also call ctr.Create() to make the container in runc?

	fmt.Printf("%s\n", ctr.ID())

	return nil
}
