package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	is "github.com/containers/image/storage"
	"github.com/containers/storage"
	units "github.com/docker/go-units"
	"github.com/kubernetes-incubator/cri-o/cmd/kpod/formats"
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
	ID        string `json:"id"`
	Created   string `json:"created"`
	CreatedBy string `json:"createdby"`
	Size      string `json:"size"`
	Comment   string `json:"comment"`
}

// historyOptions stores cli flag values
type historyOptions struct {
	image    string
	human    bool
	truncate bool
	quiet    bool
	format   string
}

var (
	historyFlags = []cli.Flag{
		cli.BoolFlag{
			Name:  "human, H",
			Usage: "Display sizes and dates in human readable format",
		},
		cli.BoolFlag{
			Name:  "no-trunc, notruncate",
			Usage: "Do not truncate the output",
		},
		cli.BoolFlag{
			Name:  "quiet, q",
			Usage: "Display the numeric IDs only",
		},
		cli.StringFlag{
			Name:  "format",
			Usage: "Change the output to JSON or a Go template",
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

	truncate := true
	if c.IsSet("no-trunc") {
		truncate = !c.Bool("no-trunc")
	}
	quiet := false
	if c.IsSet("quiet") {
		quiet = c.Bool("quiet")
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
		image:    imgName,
		human:    human,
		truncate: truncate,
		quiet:    quiet,
		format:   format,
	}
	err = outputHistory(store, opts)
	return err
}

func genHistoryFormat(quiet, truncate, human bool) (format string) {
	if quiet {
		return formats.IDString
	}

	if truncate {
		format = "table {{ .ID | printf \"%-12.12s\" }} {{ .Created | printf \"%-16s\" }} {{ .CreatedBy | " +
			"printf \"%-45.45s\" }} {{ .Size | printf \"%-16s\" }} {{ .Comment | printf \"%s\" }}"
	} else {
		format = "table {{ .ID | printf \"%-64s\" }} {{ .Created | printf \"%-18s\" }} {{ .CreatedBy | " +
			"printf \"%-60s\" }} {{ .Size | printf \"%-16s\" }} {{ .Comment | printf \"%s\"}}"
	}
	return
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
		size        int64
		img         *storage.Image
		imageID     string
		layerAll    []historyOutputParams
		createdTime string
		outputSize  string
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

		if opts.human {
			createdTime = outputHumanTime(history[i].Created) + " ago"
			outputSize = units.HumanSize(float64(size))
		} else {
			createdTime = outputTime(history[i].Created)
			outputSize = strconv.FormatInt(size, 10)
		}
		params := historyOutputParams{
			ID:        imageID,
			Created:   createdTime,
			CreatedBy: history[i].CreatedBy,
			Size:      outputSize,
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

// historyToGeneric makes an empty array of interfaces for output
func historyToGeneric(params []historyOutputParams) []interface{} {
	genericParams := make([]interface{}, len(params))
	for i, v := range params {
		genericParams[i] = interface{}(v)
	}
	return genericParams
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

	historyOutput := []historyOutputParams{}

	historyFormat := opts.format
	if historyFormat == "" {
		historyFormat = genHistoryFormat(opts.quiet, opts.truncate, opts.human)
	}

	for i := 0; i < len(history); i++ {
		imageID = history[i].ID

		outputCreatedBy = strings.Join(strings.Fields(history[i].CreatedBy), " ")
		if opts.truncate && len(outputCreatedBy) > createdByTruncLength {
			outputCreatedBy = outputCreatedBy[:createdByTruncLength-3] + "..."
		}

		if opts.truncate && i == 0 {
			imageID = history[i].ID[:idTruncLength]
		}

		params := historyOutputParams{
			ID:        imageID,
			Created:   history[i].Created,
			CreatedBy: outputCreatedBy,
			Size:      history[i].Size,
			Comment:   history[i].Comment,
		}
		historyOutput = append(historyOutput, params)
	}

	var out formats.Writer
	switch opts.format {
	case formats.JSONString:
		out = formats.JSONStructArray{Output: historyToGeneric(historyOutput)}
	default:
		out = formats.StdoutTemplateArray{Output: historyToGeneric(historyOutput), Template: historyFormat, Fields: historyOutput[0].headerMap()}

	}
	formats.Writer(out).Out()
	return nil
}

func (h *historyOutputParams) headerMap() map[string]string {
	v := reflect.Indirect(reflect.ValueOf(h))
	values := make(map[string]string)
	for h := 0; h < v.NumField(); h++ {
		key := v.Type().Field(h).Name
		value := key
		values[key] = fmt.Sprintf("%s        ", strings.ToUpper(splitCamelCase(value)))
	}
	return values
}
