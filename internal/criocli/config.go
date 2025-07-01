package criocli

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"

	"github.com/cri-o/cri-o/pkg/config"
)

var ConfigCommand = &cli.Command{
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

		if val, ok := os.LookupEnv("CONTAINER_INCLUDED_POD_METRCIS") ; ok {

			logrus.Warn("Environment variable CONTAINER_INCLUDED_POD_METRCIS is deprecated (typo). Use CONTAINER_INCLUDED_POD_METRICS instead.")

			if _, exists := os.LookupEnv("CONTAINER_INCLUDED_POD_METRICS") ; !exists {
				os.Setenv("CONTAINER_INCLUDED_POD_METRICS", val)
			}

		}



		logrus.SetFormatter(&logrus.TextFormatter{
			DisableTimestamp: true,
		})
		logrus.SetLevel(logrus.InfoLevel)

		conf, err := GetConfigFromContext(c)
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
		return conf.WriteTemplate(c.Bool("default"), os.Stdout)
	},
}
