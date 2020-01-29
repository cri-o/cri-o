package main

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/version"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

const jsonFlag = "json"

var versionCommand = &cli.Command{
	Name:  "version",
	Usage: "display detailed version information",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    jsonFlag,
			Aliases: []string{"j"},
			Usage:   "print JSON instead of text",
		},
	},
	Action: func(c *cli.Context) error {
		v := version.Get()
		res := v.String()
		if c.Bool(jsonFlag) {
			j, err := v.JSONString()
			if err != nil {
				return errors.Wrap(err, "unable to generate JSON from version info")
			}
			res = j

		}
		fmt.Println(res)
		return nil
	},
}
