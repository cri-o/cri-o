package main

import (
	"fmt"
	"os"

	"github.com/cri-o/cri-o/internal/config/migrate"
	"github.com/cri-o/cri-o/internal/criocli"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var from string

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
		&cli.StringFlag{
			Name:        "migrate-defaults",
			Aliases:     []string{"m"},
			Destination: &from,
			Usage: fmt.Sprintf(`Migrate the default config from a specified version.
    To run a config migration, just select the input config via the global
    '--config,-c' command line argument, for example:
    `+"```"+`
    crio -c /etc/crio/crio.conf.d/00-default.conf config -m 1.17
    `+"```"+`
    The migration will print converted configuration options to stderr and will
    output the resulting configuration to stdout.
    Please note that the migration will overwrite any fields that have changed
    defaults between versions. To save a custom configuration change, it should
    be in a drop-in configuration file instead.
    Possible values: %q`, migrate.From1_17),
			Value: migrate.FromPrevious,
		},
	},
	Action: func(c *cli.Context) error {
		logrus.SetFormatter(&logrus.TextFormatter{
			DisableTimestamp: true,
		})
		logrus.SetLevel(logrus.InfoLevel)

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

		if c.IsSet("migrate-defaults") {
			logrus.Infof("Migrating config from %s", from)
			if err := migrate.Config(conf, from); err != nil {
				return errors.Wrap(err, "migrate config")
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
