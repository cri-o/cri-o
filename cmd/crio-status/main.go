package main

import (
	"fmt"
	"os"

	"github.com/cri-o/cri-o/internal/criocli"
	"github.com/cri-o/cri-o/internal/version"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

const (
	defaultSocket = "/var/run/crio/crio.sock"
	idArg         = "id"
	socketArg     = "socket"

	deprecatedNotice = "\n[Deprecation Warning: This command has been deprecated and will be removed in future versions. Please use `crio status` instead.]"
)

func main() {
	app := cli.NewApp()
	app.Name = "crio-status"
	app.Authors = []*cli.Author{{Name: "The CRI-O Maintainers"}}
	app.Usage = "A tool for CRI-O status retrieval" + deprecatedNotice
	app.Description = app.Usage

	i, err := version.Get(false)
	if err != nil {
		logrus.Fatal(err)
	}
	app.Version = i.Version

	app.CommandNotFound = func(*cli.Context, string) { os.Exit(1) }
	app.OnUsageError = func(c *cli.Context, e error, b bool) error { return e }
	app.Action = func(c *cli.Context) error {
		return fmt.Errorf("expecting a valid subcommand" + deprecatedNotice)
	}

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:      socketArg,
			Aliases:   []string{"s"},
			Usage:     "absolute path to the unix socket",
			Value:     defaultSocket,
			TakesFile: true,
		},
	}
	app.Commands = criocli.DefaultCommands
	app.Commands = append(app.Commands, criocli.StatusCommand.Subcommands...)

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
