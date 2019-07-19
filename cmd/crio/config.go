package main

import (
	"os"

	"github.com/containers/image/types"
	"github.com/cri-o/cri-o/internal/lib/config"
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
		var err error
		// At this point, app.Before has already parsed the user's chosen
		// config file. So no need to handle that here.
		conf := c.App.Metadata["config"].(*config.Config) // nolint: errcheck
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
