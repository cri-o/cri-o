package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"os"

	"strconv"

	is "github.com/containers/image/storage"
	"github.com/containers/storage"
	units "github.com/docker/go-units"
	"github.com/kubernetes-incubator/cri-o/libkpod/common"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

const (
	createdByTruncLength = 45
	idTruncLength        = 13
)

// historyOutputParams stores info about each layer
type historyOutputParams struct {
	ID        string     `json:"id"`
	Created   *time.Time `json:"created"`
	CreatedBy string     `json:"createdby"`
	Size      int64      `json:"size"`
	Comment   string     `json:"comment"`
}

// historyOptions stores cli flag values
type historyOptions struct {
	image   string
	human   bool
	noTrunc bool
	quiet   bool
	format  string
}

var (
	historyFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "human, H",
			Usage: "Display sizes and dates in human readable format",
		},
		cli.BoolFlag{
			Name:  "no-trunc",
			Usage: "Do not truncate the output",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "Display the numeric IDs only",
		},
		cli.StringFlag{
			Name:  "format",
			Usage: "Pretty-print history of the image using a Go template",
		},
		cli.BoolFlag{
			Name:  "json",
			Usage: "Print the history in JSON format",
		},
	}

	historyDescription = "Displays the history of an image. The information can be printed out in an easy to read, " +
		"or user specified format, and can be truncated."
	historyCommand = cli.Command{
		Name:        "history",
		Usage:       "Show history of a specified image",
		Description: historyDescription,
		Flags:       historyFlags,
		Action:      historyCmd,
		ArgsUsage:   "",
	}
)

func historyCmd(c *cli.Context) error {
	config, err := getConfig(c)
	if err != nil {
		return errors.Wrapf(err, "Could not get config")
	}
	store, err := getStore(config)
	if err != nil {
		return err
	}

	human := true
	if c.IsSet("human") {
		human = c.Bool("human")
	}
	noTruncate := false
	if c.IsSet("no-trunc") {
		noTruncate = c.Bool("no-trunc")
	}
	quiet := false
	if c.IsSet("quiet") {
		quiet = c.Bool("quiet")
	}
	json := false
	if c.IsSet("json") {
		json = c.Bool("json")
	}
	format := ""
	if c.IsSet("format") {
		format = c.String("format")
	}

	args := c.Args()
	if len(args) == 0 {
		logrus.Errorf("an image name must be specified")
		return nil
	}
	if len(args) > 1 {
		logrus.Errorf("Kpod history takes at most 1 argument")
		return nil
	}
	imgName := args[0]

	opts := historyOptions{
		image:   imgName,
		human:   human,
		noTrunc: noTruncate,
		quiet:   quiet,
		format:  format,
	}

	var history []byte
	if json {
		history, err = createJSON(store, opts)
		fmt.Println(string(history))
	} else {
		if format == "" && !quiet {
			outputHeading(noTruncate)
		}
		err = outputHistory(store, opts)
	}
	return err
}

// outputHeader outputs the heading
func outputHeading(noTrunc bool) {
	if !noTrunc {
		fmt.Printf("%-12s\t\t%-16s\t\t%-45s\t\t", "IMAGE", "CREATED", "CREATED BY")
		fmt.Printf("%-16s\t\t%s\n", "SIZE", "COMMENT")
	} else {
		fmt.Printf("%-64s\t%-18s\t%-60s\t", "IMAGE", "CREATED", "CREATED BY")
		fmt.Printf("%-16s\t%s\n", "SIZE", "COMMENT")
	}
}

// outputString outputs the information in historyOutputParams
func outputString(noTrunc, human bool, params historyOutputParams) {
	var (
		createdTime string
		outputSize  string
	)

	if human {
		createdTime = outputHumanTime(params.Created) + " ago"
		outputSize = units.HumanSize(float64(params.Size))
	} else {
		createdTime = outputTime(params.Created)
		outputSize = strconv.FormatInt(params.Size, 10)
	}

	if !noTrunc {
		fmt.Printf("%-12.12s\t\t%-16s\t\t%-45.45s\t\t", params.ID, createdTime, params.CreatedBy)
		fmt.Printf("%-16s\t\t%s\n", outputSize, params.Comment)
	} else {
		fmt.Printf("%-64s\t%-18s\t%-60s\t", params.ID, createdTime, params.CreatedBy)
		fmt.Printf("%-16s\t%s\n\n", outputSize, params.Comment)
	}
}

// outputWithTemplate is called when --format is given a template
func outputWithTemplate(format string, params historyOutputParams, human bool) error {
	templ, err := template.New("history").Parse(format)
	if err != nil {
		return errors.Wrapf(err, "error parsing template")
	}

	createdTime := outputTime(params.Created)
	outputSize := strconv.FormatInt(params.Size, 10)

	if human {
		createdTime = outputHumanTime(params.Created) + " ago"
		outputSize = units.HumanSize(float64(params.Size))
	}

	// templParams is used to store the info from params and the time and
	// size that have been converted to type string for when the human flag
	// is set
	templParams := struct {
		ID        string
		Created   string
		CreatedBy string
		Size      string
		Comment   string
	}{
		params.ID,
		createdTime,
		params.CreatedBy,
		outputSize,
		params.Comment,
	}

	if err = templ.Execute(os.Stdout, templParams); err != nil {
		return err
	}
	fmt.Println()
	return nil
}

// outputTime displays the time stamp in "2017-06-20T20:24:10Z" format
func outputTime(tm *time.Time) string {
	return tm.Format(time.RFC3339)
}

// outputHumanTime displays the time elapsed since creation
func outputHumanTime(tm *time.Time) string {
	return units.HumanDuration(time.Since(*tm))
}

// createJSON retrieves the history of the image and returns a JSON object
func createJSON(store storage.Store, opts historyOptions) ([]byte, error) {
	var (
		size     int64
		img      *storage.Image
		imageID  string
		layerAll []historyOutputParams
	)

	ref, err := is.Transport.ParseStoreReference(store, opts.image)
	if err != nil {
		return nil, errors.Errorf("error parsing reference to image %q: %v", opts.image, err)
	}

	img, err = is.Transport.GetStoreImage(store, ref)
	if err != nil {
		return nil, errors.Errorf("no such image %q: %v", opts.image, err)
	}

	systemContext := common.GetSystemContext("")

	src, err := ref.NewImage(systemContext)
	if err != nil {
		return nil, errors.Errorf("error instantiating image %q: %v", opts.image, err)
	}

	oci, err := src.OCIConfig()
	if err != nil {
		return nil, err
	}

	history := oci.History
	layers := src.LayerInfos()
	count := 1
	// iterating backwards to get newwest to oldest
	for i := len(history) - 1; i >= 0; i-- {
		if i == len(history)-1 {
			imageID = img.ID
		} else {
			imageID = "<missing>"
		}

		if !history[i].EmptyLayer {
			size = layers[len(layers)-count].Size
			count++
		} else {
			size = 0
		}

		params := historyOutputParams{
			ID:        imageID,
			Created:   history[i].Created,
			CreatedBy: history[i].CreatedBy,
			Size:      size,
			Comment:   history[i].Comment,
		}

		layerAll = append(layerAll, params)
	}

	output, err := json.MarshalIndent(layerAll, "", "\t\t")
	if err != nil {
		return nil, errors.Errorf("error marshalling to JSON: %v", err)
	}

	if err = src.Close(); err != nil {
		return nil, err
	}

	return output, nil
}

// outputHistory gets the history of the image from the JSON object
// and pretty prints it to the screen
func outputHistory(store storage.Store, opts historyOptions) error {
	var (
		outputCreatedBy string
		imageID         string
		history         []historyOutputParams
	)

	raw, err := createJSON(store, opts)
	if err != nil {
		return errors.Errorf("error creating JSON: %v", err)
	}

	if err = json.Unmarshal(raw, &history); err != nil {
		return errors.Errorf("error Unmarshalling JSON: %v", err)
	}

	for i := 0; i < len(history); i++ {
		imageID = history[i].ID

		outputCreatedBy = strings.Join(strings.Fields(history[i].CreatedBy), " ")
		if !opts.noTrunc && len(outputCreatedBy) > createdByTruncLength {
			outputCreatedBy = outputCreatedBy[:createdByTruncLength-3] + "..."
		}

		if !opts.noTrunc && i == 0 {
			imageID = history[i].ID[:idTruncLength]
		}

		if opts.quiet {
			if !opts.noTrunc {
				fmt.Printf("%-12.12s\n", imageID)
			} else {
				fmt.Printf("%-s\n", imageID)
			}
			continue
		}

		params := historyOutputParams{
			ID:        imageID,
			Created:   history[i].Created,
			CreatedBy: outputCreatedBy,
			Size:      history[i].Size,
			Comment:   history[i].Comment,
		}

		if len(opts.format) > 0 {
			if err = outputWithTemplate(opts.format, params, opts.human); err != nil {
				return errors.Errorf("error outputing with template: %v", err)
			}
			continue
		}

		outputString(opts.noTrunc, opts.human, params)
	}
	return nil
}
