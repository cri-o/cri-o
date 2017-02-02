package main

import (
	"fmt"
	"strings"

	"github.com/urfave/cli"

	"github.com/opencontainers/runtime-tools/validate"
)

var bundleValidateFlags = []cli.Flag{
	cli.StringFlag{Name: "path", Value: ".", Usage: "path to a bundle"},
}

var bundleValidateCommand = cli.Command{
	Name:   "validate",
	Usage:  "validate an OCI bundle",
	Flags:  bundleValidateFlags,
	Before: before,
	Action: func(context *cli.Context) error {
		inputPath := context.String("path")
		hostSpecific := context.GlobalBool("host-specific")
		v, err := validate.NewValidatorFromPath(inputPath, hostSpecific)
		if err != nil {
			return err
		}

		errMsgs := v.CheckAll()
		if len(errMsgs) > 0 {
			return fmt.Errorf("%d Errors detected:\n%s", len(errMsgs), strings.Join(errMsgs, "\n"))

		}
		fmt.Println("Bundle validation succeeded.")
		return nil
	},
}
