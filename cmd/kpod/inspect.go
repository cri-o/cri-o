package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/template"

	"github.com/kubernetes-incubator/cri-o/libpod"
	libpodimage "github.com/kubernetes-incubator/cri-o/libpod/image"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	defaultFormat = `Container: {{.Container}}
ID: {{.ContainerID}}
`
	inspectTypeContainer = "container"
	inspectTypeImage     = "image"
	inspectAll           = "all"
)

var (
	inspectFlags = []cli.Flag{
		cli.StringFlag{
			Name:  "type, t",
			Value: inspectAll,
			Usage: "Return JSON for specified type, (e.g image, container or task)",
		},
		cli.StringFlag{
			Name:  "format, f",
			Value: defaultFormat,
			Usage: "Format the output using the given go template",
		},
		cli.BoolFlag{
			Name:  "size",
			Usage: "Display total file size if the type is container",
		},
	}
	inspectDescription = "This displays the low-level information on containers and images identified by name or ID. By default, this will render all results in a JSON array. If the container and image have the same name, this will return container JSON for unspecified type."
	inspectCommand     = cli.Command{
		Name:        "inspect",
		Usage:       "Displays the configuration of a container or image",
		Description: inspectDescription,
		Flags:       inspectFlags,
		Action:      inspectCmd,
		ArgsUsage:   "CONTAINER-OR-IMAGE",
	}
)

func inspectCmd(c *cli.Context) error {
	args := c.Args()
	if len(args) == 0 {
		return errors.Errorf("container or image name must be specified: kpod inspect [options [...]] name")
	}
	if len(args) > 1 {
		return errors.Errorf("too many arguments specified")
	}

	itemType := c.String("type")
	size := c.Bool("size")
	format := defaultFormat
	if c.String("format") != "" {
		format = c.String("format")
	}

	switch itemType {
	case inspectTypeContainer:
	case inspectTypeImage:
	case inspectAll:
	default:
		return errors.Errorf("the only recognized types are %q, %q, and %q", inspectTypeContainer, inspectTypeImage, inspectAll)
	}

	t := template.Must(template.New("format").Parse(format))

	name := args[0]

	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "Could not get config")
	}
	server, err := libpod.New(config)
	if err != nil {
		return errors.Wrapf(err, "could not get container server")
	}
	if err = server.Update(); err != nil {
		return errors.Wrapf(err, "could not update list of containers")
	}

	var data interface{}
	switch itemType {
	case inspectTypeContainer:
		data, err = server.GetContainerData(name, size)
		if err != nil {
			return errors.Wrapf(err, "error parsing container data")
		}
	case inspectTypeImage:
		data, err = libpodimage.GetImageData(server.Store(), name)
		if err != nil {
			return errors.Wrapf(err, "error parsing image data")
		}
	case inspectAll:
		ctrData, err := server.GetContainerData(name, size)
		if err != nil {
			imgData, err := libpodimage.GetImageData(server.Store(), name)
			if err != nil {
				return errors.Wrapf(err, "error parsing container or image data")
			}
			data = imgData

		} else {
			data = ctrData
		}
	}

	if c.IsSet("format") {
		if err = t.Execute(os.Stdout, data); err != nil {
			return err
		}
		fmt.Println()
		return nil
	}

	d, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return errors.Wrapf(err, "error encoding build container as json")
	}
	_, err = fmt.Println(string(d))
	return err
}
