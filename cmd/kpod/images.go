package main

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	is "github.com/containers/image/storage"
	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/libkpod/image"
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

type filterParams struct {
	dangling         string
	label            string
	beforeImage      string // Images are sorted by date, so we can just output until we see the image
	sinceImage       string // Images are sorted by date, so we can just output until we don't see the image
	seenImage        bool   // Hence this boolean
	referencePattern string
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

	images, err := store.Images()
	if err != nil {
		return errors.Wrapf(err, "error reading images")
	}

	var params *filterParams
	if c.IsSet("filter") {
		params, err = parseFilter(images, c.String("filter"))
		if err != nil {
			return errors.Wrapf(err, "error parsing filter")
		}
	} else {
		params = nil
	}

	if len(images) > 0 && !noheading && !quiet && !hasTemplate {
		outputHeader(truncate, digests)
	}

	return outputImages(images, formatString, store, params, name, hasTemplate, truncate, digests, quiet)
}

func parseFilter(images []storage.Image, filter string) (*filterParams, error) {
	params := new(filterParams)
	filterStrings := strings.Split(filter, ",")
	for _, param := range filterStrings {
		pair := strings.SplitN(param, "=", 2)
		switch strings.TrimSpace(pair[0]) {
		case "dangling":
			if isValidBool(pair[1]) {
				params.dangling = pair[1]
			} else {
				return nil, fmt.Errorf("invalid filter: '%s=[%s]'", pair[0], pair[1])
			}
		case "label":
			params.label = pair[1]
		case "before":
			if imageExists(images, pair[1]) {
				params.beforeImage = pair[1]
			} else {
				return nil, fmt.Errorf("no such id: %s", pair[0])
			}
		case "since":
			if imageExists(images, pair[1]) {
				params.sinceImage = pair[1]
			} else {
				return nil, fmt.Errorf("no such id: %s``", pair[0])
			}
		case "reference":
			params.referencePattern = pair[1]
		default:
			return nil, fmt.Errorf("invalid filter: '%s'", pair[0])
		}
	}
	return params, nil
}

func imageExists(images []storage.Image, ref string) bool {
	for _, image := range images {
		if matchesID(image.ID, ref) {
			return true
		}
		for _, name := range image.Names {
			if matchesReference(name, ref) {
				return true
			}
		}
	}
	return false
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

func outputImages(images []storage.Image, format string, store storage.Store, filters *filterParams, argName string, hasTemplate, truncate, digests, quiet bool) error {
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
		size, _ := image.Size(store, img)

		names := []string{""}
		if len(img.Names) > 0 {
			names = img.Names
		} else {
			// images without names should be printed with "<none>" as the image name
			names = append(names, "<none>")
		}
		for _, name := range names {
			if !matchesFilter(img, store, name, filters) || !matchesReference(name, argName) {
				continue
			}
			if quiet {
				fmt.Printf("%-64s\n", img.ID)
				// We only want to print each id once
				break
			}

			params := imageOutputParams{
				ID:        img.ID,
				Name:      name,
				Digest:    digest,
				CreatedAt: createdTime,
				Size:      formattedSize(size),
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
	}
	return nil
}

func matchesFilter(image storage.Image, store storage.Store, name string, params *filterParams) bool {
	if params == nil {
		return true
	}
	if params.dangling != "" && !matchesDangling(name, params.dangling) {
		return false
	} else if params.label != "" && !matchesLabel(image, store, params.label) {
		return false
	} else if params.beforeImage != "" && !matchesBeforeImage(image, name, params) {
		return false
	} else if params.sinceImage != "" && !matchesSinceImage(image, name, params) {
		return false
	} else if params.referencePattern != "" && !matchesReference(name, params.referencePattern) {
		return false
	}
	return true
}

func matchesDangling(name string, dangling string) bool {
	if isFalse(dangling) && name != "<none>" {
		return true
	} else if isTrue(dangling) && name == "<none>" {
		return true
	}
	return false
}

func matchesLabel(image storage.Image, store storage.Store, label string) bool {
	storeRef, err := is.Transport.ParseStoreReference(store, "@"+image.ID)
	if err != nil {

	}
	img, err := storeRef.NewImage(nil)
	if err != nil {
		return false
	}
	info, err := img.Inspect()
	if err != nil {
		return false
	}

	pair := strings.SplitN(label, "=", 2)
	for key, value := range info.Labels {
		if key == pair[0] {
			if len(pair) == 2 {
				if value == pair[1] {
					return true
				}
			} else {
				return false
			}
		}
	}
	return false
}

// Returns true if the image was created since the filter image.  Returns
// false otherwise
func matchesBeforeImage(image storage.Image, name string, params *filterParams) bool {
	if params.seenImage {
		return false
	}
	if matchesReference(name, params.beforeImage) || matchesID(image.ID, params.beforeImage) {
		params.seenImage = true
		return false
	}
	return true
}

// Returns true if the image was created since the filter image.  Returns
// false otherwise
func matchesSinceImage(image storage.Image, name string, params *filterParams) bool {
	if params.seenImage {
		return true
	}
	if matchesReference(name, params.sinceImage) || matchesID(image.ID, params.sinceImage) {
		params.seenImage = true
	}
	return false
}

func matchesID(id, argID string) bool {
	return strings.HasPrefix(argID, id)
}

func matchesReference(name, argName string) bool {
	if argName == "" {
		return true
	}
	splitName := strings.Split(name, ":")
	// If the arg contains a tag, we handle it differently than if it does not
	if strings.Contains(argName, ":") {
		splitArg := strings.Split(argName, ":")
		return strings.HasSuffix(splitName[0], splitArg[0]) && (splitName[1] == splitArg[1])
	}
	return strings.HasSuffix(splitName[0], argName)
}

func formattedSize(size int64) string {
	suffixes := [5]string{"B", "KB", "MB", "GB", "TB"}

	count := 0
	formattedSize := float64(size)
	for formattedSize >= 1024 && count < 4 {
		formattedSize /= 1024
		count++
	}
	return fmt.Sprintf("%.4g %s", formattedSize, suffixes[count])
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
