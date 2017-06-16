package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	cp "github.com/containers/image/copy"
	is "github.com/containers/image/storage"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

type imageMetadata struct {
	Tag            string              `json:"tag"`
	CreatedTime    time.Time           `json:"created-time"`
	ID             string              `json:"id"`
	Blobs          []types.BlobInfo    `json:"blob-list"`
	Layers         map[string][]string `json:"layers"`
	SignatureSizes []string            `json:"signature-sizes"`
}

func getStore(c *cli.Context) (storage.Store, error) {
	options := storage.DefaultStoreOptions
	if c.GlobalIsSet("root") {
		options.GraphRoot = c.GlobalString("root")
	}
	if c.GlobalIsSet("runroot") {
		options.RunRoot = c.GlobalString("runroot")
	}

	if c.GlobalIsSet("storage-driver") {
		options.GraphDriverName = c.GlobalString("storage-driver")
	}
	if c.GlobalIsSet("storage-opt") {
		opts := c.GlobalStringSlice("storage-opt")
		if len(opts) > 0 {
			options.GraphDriverOptions = opts
		}
	}
	store, err := storage.GetStore(options)
	if err != nil {
		return nil, err
	}
	is.Transport.SetStore(store)
	return store, nil
}

func findImage(store storage.Store, image string) (*storage.Image, error) {
	var img *storage.Image
	ref, err := is.Transport.ParseStoreReference(store, image)
	if err == nil {
		img, err = is.Transport.GetStoreImage(store, ref)
	}
	if err != nil {
		img2, err2 := store.Image(image)
		if err2 != nil {
			if ref == nil {
				return nil, errors.Wrapf(err, "error parsing reference to image %q", image)
			}
			return nil, errors.Wrapf(err, "unable to locate image %q", image)
		}
		img = img2
	}
	return img, nil
}

func getCopyOptions(reportWriter io.Writer) *cp.Options {
	return &cp.Options{
		ReportWriter: reportWriter,
	}
}

func getSystemContext(signaturePolicyPath string) *types.SystemContext {
	sc := &types.SystemContext{}
	if signaturePolicyPath != "" {
		sc.SignaturePolicyPath = signaturePolicyPath
	}
	return sc
}

func parseMetadata(image storage.Image) (imageMetadata, error) {
	var im imageMetadata

	dec := json.NewDecoder(strings.NewReader(image.Metadata))
	if err := dec.Decode(&im); err != nil {
		return imageMetadata{}, err
	}
	return im, nil
}

func getSize(image storage.Image, store storage.Store) (int64, error) {

	is.Transport.SetStore(store)
	storeRef, err := is.Transport.ParseStoreReference(store, "@"+image.ID)
	if err != nil {
		fmt.Println(err)
		return -1, err
	}
	img, err := storeRef.NewImage(nil)
	if err != nil {
		fmt.Println("Error with NewImage")
		return -1, err
	}
	imgSize, err := img.Size()
	if err != nil {
		fmt.Println("Error getting size")
		return -1, err
	}
	return imgSize, nil
}
