package main

import (
	"os"

	"github.com/cri-o/cri-o/internal/pkg/criocli"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/urfave/cli/v2"
)

var configCommand = &cli.Command{
	Name: "config",
	Usage: `Outputs a commented version of the configuration file that could be used
by CRI-O. This allows you to save you current configuration setup and then load
it later with **--config**. Global options will modify the output.`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "default",
			Usage: "Output the default configuration (without taking into account any configuration options).",
		},
	},
	Action: func(c *cli.Context) error {
		conf, err := criocli.GetConfigFromContext(c)
		if err != nil {
			return err
		}

		if c.Bool("default") {
			conf, err = config.DefaultConfig()
			if err != nil {
				return err
			}
		}

		// Validate the configuration during generation
		if err = conf.Validate(false); err != nil {
			return err
		}

		// Output the commented config.
		return conf.WriteTemplate(os.Stdout)
	},
}
