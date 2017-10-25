package main

import (

	"fmt"
	"os"
	"os/exec"

	"github.com/kubernetes-incubator/cri-o/libkpod"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)


var (
	budFlags = []cli.Flag{
		cli.StringSliceFlag{
			Name:  "build-arg",
			Usage: "`argument=value` to supply to the builder",
		},
		cli.StringSliceFlag{
			Name:  "file, f",
			Usage: "`pathname or URL` of a Dockerfile",
		},
		cli.StringFlag{
			Name:  "format",
			Usage: "`format` of the built image's manifest and metadata",
		},
		cli.BoolTFlag{
			Name:  "pull",
			Usage: "pull the image if not present",
		},
		cli.BoolFlag{
			Name:  "pull-always",
			Usage: "pull the image, even if a version is present",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "refrain from announcing build instructions and image read/write progress",
		},
		cli.StringFlag{
			Name:  "runtime",
			Usage: "`path` to an alternate runtime",
		},
		cli.StringSliceFlag{
			Name:  "runtime-flag",
			Usage: "add global flags for the container runtime",
		},
		cli.StringFlag{
			Name:  "signature-policy",
			Usage: "`pathname` of signature policy file (not usually used)",
		},
		cli.StringSliceFlag{
			Name:  "tag, t",
			Usage: "`tag` to apply to the built image",
		},
	}
	budDescription = "This creates an OCI image  using the 'buildah bud' command.  Buildah must be installed for this command to work."
	budCommand     = cli.Command{
		Name:        "bud",
		Usage:       "Builds a container using the buildah bud command",
		Description: budDescription,
		Flags:       budFlags,
		Action:      budCmd,
		ArgsUsage:   "CONTEXT-DIRECTORY | URL",
	}
)

func budCmd(c *cli.Context) error {
	//args := c.Args()

	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "Could not get config")
	}
	server, err := libkpod.New(config)
	if err != nil {
		return errors.Wrapf(err, "could not get container server")
	}
	defer server.Shutdown()
	if err = server.Update(); err != nil {
		return errors.Wrapf(err, "could not update list of containers")
	}

        buildah := "buildah"



	_, err = exec.Command(buildah).Output()
	if err != nil {
		return errors.Wrapf(err, "buildah is not installed on this server")
	}

	budCmdArgs := []string{"bud"}
	if c.BoolT("pull") {
		budCmdArgs = append(budCmdArgs, "--pull")
	}

	if c.BoolT("pull-always") {
		budCmdArgs = append(budCmdArgs, "--pull-always")
	}

	fmt.Fprintln(os.Stdout, "Tom's args: ", budCmdArgs)
	budCmdArgs = append(budCmdArgs, ".")
	cmd := exec.Command(buildah, budCmdArgs...)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "There was an error running the buildah bud command: ", err)
	}
		
	return nil
}

