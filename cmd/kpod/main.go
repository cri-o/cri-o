package main

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/containers/storage/pkg/reexec"
	"github.com/urfave/cli"
)

//Version of kpod
const Version string = "0.0.1"

func main() {
	if reexec.Init() {
		return
	}

	app := cli.NewApp()
	app.Name = "kpod"
	app.Usage = "manage pods and images"
	app.Version = Version

	app.Commands = []cli.Command{
		imagesCommand,
		launchCommand,
		rmiCommand,
		stopCommand,
		tagCommand,
		versionCommand,
		pullCommand,
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "root",
			Usage: "path to the root directory in which data, including images,  is stored",
		},
		cli.StringFlag{
			Name:  "runroot",
			Usage: "path to the 'run directory' where all state information is stored",
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
		logrus.Fatal(err)
	}
}
