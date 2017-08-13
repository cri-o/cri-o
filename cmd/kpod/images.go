package main

import (
	"fmt"
	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/cmd/kpod/formats"
	libkpodimage "github.com/kubernetes-incubator/cri-o/libkpod/image"
	digest "github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type imageOutputParams struct {
	ID        string        `json:"id"`
	Name      string        `json:"names"`
	Digest    digest.Digest `json:"digest"`
	CreatedAt string        `json:"created"`
	Size      string        `json:"size"`
}

var (
	imagesFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "display only image IDs",
		},
		cli.BoolFlag{
			Name:  "noheading, n",
			Usage: "do not print column headings",
		},
		cli.BoolFlag{
			Name:  "no-trunc, notruncate",
			Usage: "do not truncate output",
		},
		cli.BoolFlag{
			Name:  "digests",
			Usage: "show digests",
		},
		cli.StringFlag{
			Name:  "format",
			Usage: "Change the output format.",
		},
		cli.StringFlag{
			Name:  "filter, f",
			Usage: "filter output based on conditions provided (default [])",
		},
	}

	imagesDescription = "lists locally stored images."
	imagesCommand     = cli.Command{
		Name:        "images",
		Usage:       "list images in local storage",
		Description: imagesDescription,
		Flags:       imagesFlags,
		Action:      imagesCmd,
		ArgsUsage:   "",
	}
)

type stdoutstruct struct {
	output                              []imageOutputParams
	truncate, digests, quiet, noheading bool
}

func (so stdoutstruct) Out() error {
	if len(so.output) > 0 && !so.noheading && !so.quiet {
		outputHeader(so.truncate, so.digests)
	}
	lastID := ""
	for _, img := range so.output {
		if so.quiet {
			if lastID == img.ID {
				continue // quiet should not show the same ID multiple times.
			}
			fmt.Printf("%-64s\n", img.ID)
			continue
		}
		if so.truncate {
			fmt.Printf("%-20.12s %-56s", img.ID, img.Name)
		} else {
			fmt.Printf("%-64s %-56s", img.ID, img.Name)
		}

		if so.digests {
			fmt.Printf(" %-64s", img.Digest)
		}
		fmt.Printf(" %-22s %s\n", img.CreatedAt, img.Size)

	}
	return nil
}

func toGeneric(params []imageOutputParams) []interface{} {
	genericParams := make([]interface{}, len(params))
	for i, v := range params {
		genericParams[i] = interface{}(v)
	}
	return genericParams
}

func imagesCmd(c *cli.Context) error {
	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "Could not get config")
	}
	store, err := getStore(config)
	if err != nil {
		return err
	}

	quiet := false
	if c.IsSet("quiet") {
		quiet = c.Bool("quiet")
	}
	noheading := false
	if c.IsSet("noheading") {
		noheading = c.Bool("noheading")
	}
	truncate := true
	if c.IsSet("no-trunc") {
		truncate = !c.Bool("no-trunc")
	}
	digests := false
	if c.IsSet("digests") {
		digests = c.Bool("digests")
	}
	outputFormat := ""
	if c.IsSet("format") {
		outputFormat = c.String("format")
	}

	name := ""
	if len(c.Args()) == 1 {
		name = c.Args().Get(0)
	} else if len(c.Args()) > 1 {
		return errors.New("'buildah images' requires at most 1 argument")
	}

	var params *libkpodimage.FilterParams
	if c.IsSet("filter") {
		params, err = libkpodimage.ParseFilter(store, c.String("filter"))
		if err != nil {
			return errors.Wrapf(err, "error parsing filter")
		}
	} else {
		params = nil
	}

	imageList, err := libkpodimage.GetImagesMatchingFilter(store, params, name)
	if err != nil {
		return errors.Wrapf(err, "could not get list of images matching filter")
	}

	return outputImages(store, imageList, truncate, digests, quiet, outputFormat, noheading)
}

func outputHeader(truncate, digests bool) {
	if truncate {
		fmt.Printf("%-20s %-56s ", "IMAGE ID", "IMAGE NAME")
	} else {
		fmt.Printf("%-64s %-56s ", "IMAGE ID", "IMAGE NAME")
	}

	if digests {
		fmt.Printf("%-71s ", "DIGEST")
	}

	fmt.Printf("%-22s %s\n", "CREATED AT", "SIZE")
}

func outputImages(store storage.Store, images []storage.Image, truncate, digests, quiet bool, outputFormat string, noheading bool) error {
	imageOutput := []imageOutputParams{}

	for _, img := range images {
		createdTime := img.Created

		name := ""
		if len(img.Names) > 0 {
			name = img.Names[0]
		}

		info, digest, size, _ := libkpodimage.InfoAndDigestAndSize(store, img)
		if info != nil {
			createdTime = info.Created
		}

		params := imageOutputParams{
			ID:        img.ID,
			Name:      name,
			Digest:    digest,
			CreatedAt: createdTime.Format("Jan 2, 2006 15:04"),
			Size:      libkpodimage.FormattedSize(size),
		}
		imageOutput = append(imageOutput, params)
	}

	var out formats.Writer

	if outputFormat != "" {
		switch outputFormat {
		case "json":
			out = formats.JSONstruct{Output: toGeneric(imageOutput)}
		default:
			// Assuming Go-template
			out = formats.StdoutTemplate{Output: toGeneric(imageOutput), Template: outputFormat}

		}
	} else {
		out = stdoutstruct{output: imageOutput, digests: digests, truncate: truncate, quiet: quiet, noheading: noheading}
	}

	formats.Writer(out).Out()

	return nil
}
