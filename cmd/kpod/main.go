package main

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/urfave/cli"
)

// TODO should share this default with ocid - move inside server, maybe?
const ociConfigPath = "/etc/ocid/ocid.conf"

func main() {
	app := cli.NewApp()
	app.Name = "kpod"
	app.Usage = "manage pods and images"
	app.Version = "0.0.1"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config",
			Value: ociConfigPath,
			Usage: "path to OCI config file",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug level log output",
		},
	}

	app.Commands = []cli.Command{
		launchCommand,
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
