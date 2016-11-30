package main

import (
	"fmt"

	"github.com/urfave/cli"
)

// TODO implement
var launchCommand = cli.Command{
	Name:  "launch",
	Usage: "launch a pod",
	Action: func(context *cli.Context) error {
		return fmt.Errorf("this functionality is not yet implemented")
	},
}
