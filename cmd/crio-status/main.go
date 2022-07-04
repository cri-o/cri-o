package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/cri-o/cri-o/internal/client"
	"github.com/cri-o/cri-o/internal/criocli"
	"github.com/cri-o/cri-o/internal/version"
	"github.com/cri-o/cri-o/pkg/types"
	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

const (
	defaultSocket = "/var/run/crio/crio.sock"
	defaultOutput = "text"
	idArg         = "id"
	socketArg     = "socket"
	outputArg     = "output"
)

func main() {
	app := cli.NewApp()
	app.Name = "crio-status"
	app.Authors = []*cli.Author{{Name: "The CRI-O Maintainers"}}
	app.Usage = "A tool for CRI-O status retrieval"
	app.Description = app.Usage

	i, err := version.Get(false)
	if err != nil {
		logrus.Fatal(err)
	}
	app.Version = i.Version

	app.CommandNotFound = func(*cli.Context, string) { os.Exit(1) }
	app.OnUsageError = func(c *cli.Context, e error, b bool) error { return e }
	app.Action = func(c *cli.Context) error {
		return fmt.Errorf("expecting a valid subcommand")
	}

	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:      socketArg,
			Aliases:   []string{"s"},
			Usage:     "absolute path to the unix socket",
			Value:     defaultSocket,
			TakesFile: true,
		},
		&cli.StringFlag{
			Name:      outputArg,
			Aliases:   []string{"o", "out"},
			Usage:     "Specify the output for operations. Can be json, yaml, or text.",
			Value:     defaultOutput,
			TakesFile: false,
		},
	}
	app.Commands = criocli.DefaultCommands
	app.Commands = append(app.Commands, []*cli.Command{{
		Action:  config,
		Aliases: []string{"c"},
		Name:    "config",
		Usage:   "Show the configuration of CRI-O as TOML string.",
	}, {
		Action:  containers,
		Aliases: []string{"container", "cs", "s"},
		Flags: []cli.Flag{&cli.StringFlag{
			Name:    idArg,
			Aliases: []string{"i"},
			Usage:   "the container ID",
		}},
		Name:  "containers",
		Usage: "Display detailed information about the provided container ID.",
	}, {
		Action:  info,
		Aliases: []string{"i"},
		Name:    "info",
		Usage:   "Retrieve generic information about CRI-O, like the cgroup and storage driver.",
	}}...)

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func config(c *cli.Context) error {
	crioClient, err := crioClient(c)
	if err != nil {
		return err
	}

	info, err := crioClient.ConfigInfo()
	if err != nil {
		return err
	}

	fmt.Print(info)
	return nil
}

func containers(c *cli.Context) error {
	crioClient, err := crioClient(c)
	if err != nil {
		return err
	}

	id := c.String(idArg)
	if id == "" {
		return fmt.Errorf("the argument --%s cannot be empty", idArg)
	}

	info, err := crioClient.ContainerInfo(c.String(idArg))
	if err != nil {
		return err
	}

	if output, e := genOutput(info); e == nil {
		fmt.Print(output)
		return nil
	} else {
		return e
	}
}

func info(c *cli.Context) error {
	crioClient, err := crioClient(c)
	if err != nil {
		return err
	}

	info, err := crioClient.DaemonInfo()
	if err != nil {
		return err
	}

	output, err1 := genOutput(&info)
	if err1 != nil {
		return err1
	}

	fmt.Print(output)
	return nil
}

func genOutput(input types.Output) (string, error) {
	switch outputArg {
	case "text":
		return input.MarshalText(), nil
	case "json":
		str, e := json.Marshal(&input)
		return string(str), e
	case "yaml":
		str, e := yaml.Marshal(&input)
		return string(str), e
	default:
		return "", errors.New("invalid output type specified, must be json, yaml, or text")
	}
}

func crioClient(c *cli.Context) (client.CrioClient, error) {
	return client.New(c.String(socketArg))
}
