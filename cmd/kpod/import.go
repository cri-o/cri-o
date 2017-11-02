package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/kubernetes-incubator/cri-o/libpod"

	"github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var (
	importFlags = []cli.Flag{
		cli.StringFlag{
			Name:  "change, c",
			Usage: "Apply imgspecv1 configurations to the created image",
		},
		cli.StringFlag{
			Name:  "message, m",
			Usage: "Set commit message for image imported",
		},
	}
	importDescription = "Imports a tarball and saves it as a root filesystem image.\n" +
		"The commit message and image config can be modified by the user."
	importCommand = cli.Command{
		Name:        "import",
		Usage:       "Import a tarball to create a filesystem image",
		Description: importDescription,
		Flags:       importFlags,
		Action:      importCmd,
		ArgsUsage:   "TARBALL",
	}
)

func importCmd(c *cli.Context) error {
	if err := validateFlags(c, historyFlags); err != nil {
		return err
	}

	runtime, err := getRuntime(c)
	if err != nil {
		return errors.Wrapf(err, "Could not get runtime")
	}
	defer runtime.Shutdown(false)

	var opts libpod.CopyOptions
	var source string
	args := c.Args()
	switch len(args) {
	case 0:
		return errors.Errorf("need to give the path to the tarball")
	case 1:
		source = args[0]
	case 2:
		source = args[0]
		opts.Reference = args[1]
	default:
		return errors.Errorf("too many arguments, need 2 only")
	}

	if _, err := url.ParseRequestURI(source); err == nil {
		file, err := downloadFromURL(source)
		if err != nil {
			return nil
		}
		defer os.Remove(file)
		source = file
	}

	changes := v1.ImageConfig{}
	if c.IsSet("change") {
		changes, err = getImageConfig(c.String("change"))
		if err != nil {
			return errors.Wrapf(err, "error adding config changes to image %q", source)
		}
	}

	history := []v1.History{
		{Comment: c.String("message")},
	}

	config := v1.Image{
		Config:  changes,
		History: history,
	}

	opts.ImageConfig = config

	return runtime.ImportImage(source, opts)
}

// donwloadFromURL downloads an image in the format "https:/example.com/myimage.tar"
// and tempoarily saves in it /var/tmp/importxyz, which is deleted after the image is imported
func downloadFromURL(source string) (string, error) {
	fmt.Printf("Downloading from %q\n", source)

	outFile, err := ioutil.TempFile("/var/tmp", "import")
	if err != nil {
		return "", errors.Wrap(err, "error creating file")
	}
	defer outFile.Close()

	response, err := http.Get(source)
	if err != nil {
		return "", errors.Wrapf(err, "error downloading %q", source)
	}
	defer response.Body.Close()

	_, err = io.Copy(outFile, response.Body)
	if err != nil {
		return "", errors.Wrapf(err, "error saving %q to %q", source, outFile)
	}

	return outFile.Name(), nil
}

// getImageConfig converts the --change flag values in the format "CMD=/bin/bash USER=example"
// to a type v1.ImageConfig
func getImageConfig(change string) (v1.ImageConfig, error) {
	// USER=value | EXPOSE=value | ENV=value | ENTRYPOINT=value |
	// CMD=value | VOLUME=value | WORKDIR=value | LABEL=key=value | STOPSIGNAL=value

	var (
		user       string
		env        []string
		entrypoint []string
		cmd        []string
		workingDir string
		stopSignal string
	)

	exposedPorts := make(map[string]struct{})
	volumes := make(map[string]struct{})
	labels := make(map[string]string)

	changes := strings.Split(change, " ")

	for _, ch := range changes {
		pair := strings.Split(ch, "=")
		if len(pair) == 1 {
			return v1.ImageConfig{}, errors.Errorf("no value given for instruction %q", ch)
		}
		switch pair[0] {
		case "USER":
			user = pair[1]
		case "EXPOSE":
			var st struct{}
			exposedPorts[pair[1]] = st
		case "ENV":
			env = append(env, pair[1])
		case "ENTRYPOINT":
			entrypoint = append(entrypoint, pair[1])
		case "CMD":
			cmd = append(cmd, pair[1])
		case "VOLUME":
			var st struct{}
			volumes[pair[1]] = st
		case "WORKDIR":
			workingDir = pair[1]
		case "LABEL":
			if len(pair) == 3 {
				labels[pair[1]] = pair[2]
			} else {
				labels[pair[1]] = ""
			}
		case "STOPSIGNAL":
			stopSignal = pair[1]
		}
	}

	return v1.ImageConfig{
		User:         user,
		ExposedPorts: exposedPorts,
		Env:          env,
		Entrypoint:   entrypoint,
		Cmd:          cmd,
		Volumes:      volumes,
		WorkingDir:   workingDir,
		Labels:       labels,
		StopSignal:   stopSignal,
	}, nil
}
