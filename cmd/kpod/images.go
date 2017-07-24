package main

import (
	"fmt"
	"os"
	"text/template"

	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/libkpod/image"
	libkpodimage "github.com/kubernetes-incubator/cri-o/libkpod/image"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type imageOutputParams struct {
	ID        string
	Name      string
	Digest    string
	CreatedAt string
	Size      string
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
			Usage: "pretty-print images using a Go template. will override --quiet",
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

func imagesCmd(c *cli.Context) error {
	store, err := getStore(c)
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
	formatString := ""
	hasTemplate := false
	if c.IsSet("format") {
		formatString = c.String("format")
		hasTemplate = true
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
	if len(imageList) > 0 && !noheading && !quiet && !hasTemplate {
		outputHeader(truncate, digests)
	}

	return outputImages(store, imageList, formatString, hasTemplate, truncate, digests, quiet)
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

func outputImages(store storage.Store, images []storage.Image, format string, hasTemplate, truncate, digests, quiet bool) error {
	for _, img := range images {
		imageMetadata, err := image.ParseMetadata(img)
		if err != nil {
			fmt.Println(err)
		}
		createdTime := imageMetadata.CreatedTime.Format("Jan 2, 2006 15:04")
		digest := ""
		if len(imageMetadata.Blobs) > 0 {
			digest = string(imageMetadata.Blobs[0].Digest)
		}
		size, _ := libkpodimage.Size(store, img)

		if quiet {
			fmt.Printf("%-64s\n", img.ID)
			// We only want to print each id once
			break
		}

		params := imageOutputParams{
			ID:        img.ID,
			Name:      img.Names[0],
			Digest:    digest,
			CreatedAt: createdTime,
			Size:      libkpodimage.FormattedSize(size),
		}
		if hasTemplate {
			err = outputUsingTemplate(format, params)
			if err != nil {
				return err
			}
			continue
		}
		outputUsingFormatString(truncate, digests, params)
	}
	return nil
}

func outputUsingTemplate(format string, params imageOutputParams) error {
	tmpl, err := template.New("image").Parse(format)
	if err != nil {
		return errors.Wrapf(err, "Template parsing error")
	}

	err = tmpl.Execute(os.Stdout, params)
	if err != nil {
		return err
	}
	fmt.Println()
	return nil
}

func outputUsingFormatString(truncate, digests bool, params imageOutputParams) {
	if truncate {
		fmt.Printf("%-20.12s %-56s", params.ID, params.Name)
	} else {
		fmt.Printf("%-64s %-56s", params.ID, params.Name)
	}

	if digests {
		fmt.Printf(" %-64s", params.Digest)
	}
	fmt.Printf(" %-22s %s\n", params.CreatedAt, params.Size)
}
