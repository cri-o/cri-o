package main

import (
	"os"

	"github.com/containers/image/v5/types"
	"github.com/cri-o/cri-o/internal/lib/config"
	"github.com/cri-o/cri-o/internal/pkg/criocli"
	"github.com/urfave/cli"
)

var configCommand = cli.Command{
	Name:  "config",
	Usage: "generate crio configuration files",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "default",
			Usage: "output the default configuration",
		},
	},
	Action: func(c *cli.Context) error {
		_, conf, err := criocli.GetConfigFromContext(c)
		if err != nil {
			return err
		}

		systemContext := &types.SystemContext{}
		if c.Bool("default") {
			conf, err = config.DefaultConfig()
			if err != nil {
				return err
			}
		}

		// Validate the configuration during generation
		if err = conf.Validate(systemContext, false); err != nil {
			return err
		}

		// Output the commented config.
		return conf.WriteTemplate(os.Stdout)
	},
}
