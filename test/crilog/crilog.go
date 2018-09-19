package main

import (
	"fmt"
	"os"

	"github.com/kubernetes-sigs/cri-o/utils"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "crilog"
	app.Usage = "simple CRI log format file parser"
	app.Version = "0.1"

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "log-path",
			Usage: "path to log file",
		},
	}

	app.Action = func(c *cli.Context) error {
		logPath := c.GlobalString("log-path")

		stdoutBytes, stderrBytes, err := utils.ParseCRILog(logPath)
		if err != nil {
			return err
		}

		fmt.Fprintf(os.Stdout, "%s", string(stdoutBytes))
		fmt.Fprintf(os.Stderr, "%s", string(stderrBytes))

		return nil
	}

	if err := app.Run(os.Args); err != nil {
		logrus.Fatal(err)
	}
}
