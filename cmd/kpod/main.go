package main

import (
	"os"

	"github.com/containers/storage/pkg/reexec"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	if reexec.Init() {
		return
	}

	app := cli.NewApp()
	app.Name = "kpod"
	app.Usage = "manage pods and images"
	app.Version = "0.0.1"

	app.Commands = []cli.Command{
		diffCommand,
		exportCommand,
		historyCommand,
		imagesCommand,
		infoCommand,
		inspectCommand,
		loadCommand,
		logsCommand,
		mountCommand,
		psCommand,
		pullCommand,
		pushCommand,
		renameCommand,
		rmCommand,
		rmiCommand,
		saveCommand,
		statsCommand,
		tagCommand,
		umountCommand,
		versionCommand,
	}
	app.Before = func(c *cli.Context) error {
		logrus.SetLevel(logrus.ErrorLevel)
		if c.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}
		return nil
	}
	app.After = func(*cli.Context) error {
		// called by Run() when the command handler succeeds
		shutdownStores()
		return nil
	}
	cli.OsExiter = func(code int) {
		// called by Run() when the command fails, bypassing After()
		shutdownStores()
		os.Exit(code)
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config, c",
			Usage: "path of a config file detailing container server configuration options",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "print debugging information",
		},
		cli.StringFlag{
			Name:  "root",
			Usage: "path to the root directory in which data, including images, is stored",
		},
		cli.StringFlag{
			Name:  "runroot",
			Usage: "path to the 'run directory' where all state information is stored",
		},
		cli.StringFlag{
			Name:  "runtime",
			Usage: "path to the OCI-compatible binary used to run containers, default is /usr/bin/runc",
		},
		cli.StringFlag{
			Name:  "storage-driver, s",
			Usage: "select which storage driver is used to manage storage of images and containers (default is overlay2)",
		},
		cli.StringSliceFlag{
			Name:  "storage-opt",
			Usage: "used to pass an option to the storage driver",
		},
	}
	if err := app.Run(os.Args); err != nil {
		logrus.Errorf(err.Error())
		cli.OsExiter(1)
	}
}
