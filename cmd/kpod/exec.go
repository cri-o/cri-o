package main

import (
	"github.com/kubernetes-incubator/cri-o/libkpod"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var (
	execFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "detach, d",
			Usage: "detached mode: command will run in the background",
		},
		cli.StringSliceFlag{
			Name:  "env, e",
			Usage: "set environment variables",
		},
		cli.BoolFlag{
			Name:  "tty, t",
			Usage: "Allocate a pseudo TTY",
		},
		cli.StringFlag{
			Name:  "user, u",
			Usage: "Sets the username (UID) and groupname (GID) for the specified command",
		},
	}
	execDescription = `
   kpod exec

   Run a command in a running container.  The command will only run while the primary
	PID in the container is running.
`

	execCommand = cli.Command{
		Name:        "exec",
		Usage:       "Run a command in a running container",
		Description: execDescription,
		Flags:       execFlags,
		Action:      execCmd,
		ArgsUsage:   "[OPTIONS} CONTAINER-NAME COMMAND",
	}
)

func execCmd(c *cli.Context) error {
	args := c.Args()
	detachFlag := c.Bool("detach")
	envFlags := c.StringSlice("env")
	ttyFlag := c.Bool("tty")
	userFlag := c.String("user")

	if len(args) < 2 {
		return errors.Errorf("you must provide a container name or ID and a command to run")
	}
	if err := validateFlags(c, execFlags); err != nil {
		return err
	}
	container := args[0]
	var command []string
	for _, cmd := range args[1:] {
		command = append(command, cmd)
	}

	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "could not get config")
	}
	server, err := libkpod.New(config)
	if err != nil {
		return errors.Wrapf(err, "could not get container server")
	}
	defer server.Shutdown()
	err = server.Update()
	if err != nil {
		return errors.Wrapf(err, "could not update list of containers")
	}
	if err := server.ContainerExec(container, command, detachFlag, envFlags, ttyFlag, userFlag); err != nil {
		return err
	}
	return nil

}
