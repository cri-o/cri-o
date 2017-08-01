package main

import (
	"encoding/json"
	"fmt"

	"github.com/containers/storage/pkg/archive"
	"github.com/kubernetes-incubator/cri-o/libkpod"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type diffJSONOutput struct {
	Changed []string `json:"changed,omitempty"`
	Added   []string `json:"added,omitempty"`
	Deleted []string `json:"deleted,omitempty"`
}

var (
	diffFlags = []cli.Flag{
		cli.BoolFlag{
			Name:   "archive",
			Usage:  "Save the diff as a tar archive",
			Hidden: true,
		},
		cli.BoolFlag{
			Name:  "json",
			Usage: "Format output as JSON",
		},
	}
	diffDescription = fmt.Sprint(`Displays changes on a container or image's filesystem.  The
	container or image will be compared to its parent layer`)

	diffCommand = cli.Command{
		Name:        "diff",
		Usage:       "Inspect changes on container's file systems",
		Description: diffDescription,
		Flags:       diffFlags,
		Action:      diffCmd,
		ArgsUsage:   "ID-NAME",
	}
)

func diffCmd(c *cli.Context) error {
	if len(c.Args()) != 1 {
		return errors.Errorf("container, layer, or image name must be specified: kpod diff [options [...]] ID-NAME")
	}
	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "could not get config")
	}

	server, err := libkpod.New(config)
	if err != nil {
		return errors.Wrapf(err, "could not get container server")
	}

	to := c.Args().Get(0)
	changes, err := server.GetDiff("", to)
	if err != nil {
		return errors.Wrapf(err, "could not get changes for %q", to)
	}

	if c.Bool("json") {
		jsonStruct := diffJSONOutput{}
		for _, change := range changes {
			if change.Kind == archive.ChangeModify {
				jsonStruct.Changed = append(jsonStruct.Changed, change.Path)
			} else if change.Kind == archive.ChangeAdd {
				jsonStruct.Added = append(jsonStruct.Added, change.Path)
			} else if change.Kind == archive.ChangeDelete {
				jsonStruct.Deleted = append(jsonStruct.Deleted, change.Path)
			} else {
				return errors.Errorf("change kind %q not recognized", change.Kind.String())
			}
		}
		b, err := json.MarshalIndent(jsonStruct, "", "  ")
		if err != nil {
			return errors.Wrapf(err, "could not marshal json for %+v", jsonStruct)
		}
		fmt.Println(string(b))
	} else {
		for _, change := range changes {
			fmt.Println(change.String())
		}
	}

	return nil
}
