package main

import (
	"strings"

	is "github.com/containers/image/storage"
	"github.com/containers/image/types"
	"github.com/containers/storage"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

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

func parseRegistryCreds(creds string) (*types.DockerAuthConfig, error) {
	if creds == "" {
		return nil, errors.New("no credentials supplied")
	}
	if strings.Index(creds, ":") < 0 {
		return nil, errors.New("user name supplied, but no password supplied")
	}
	v := strings.SplitN(creds, ":", 2)
	cfg := &types.DockerAuthConfig{
		Username: v[0],
		Password: v[1],
	}
	return cfg, nil
}
