package main

import (
	is "github.com/containers/image/storage"
	"github.com/containers/storage"
	"github.com/kubernetes-incubator/cri-o/libkpod"
	"github.com/urfave/cli"
)

func getStore(c *libkpod.Config) (storage.Store, error) {
	options := storage.DefaultStoreOptions
	options.GraphRoot = c.Root
	options.RunRoot = c.RunRoot
	options.GraphDriverName = c.Storage
	options.GraphDriverOptions = c.StorageOptions

	store, err := storage.GetStore(options)
	if err != nil {
		return nil, err
	}
	is.Transport.SetStore(store)
	return store, nil
}

func getConfig(c *cli.Context) (*libkpod.Config, error) {
	config := libkpod.DefaultConfig()
	if c.GlobalIsSet("config") {
		err := config.UpdateFromFile(c.String("config"))
		if err != nil {
			return config, err
		}
	}
	if c.GlobalIsSet("root") {
		config.Root = c.GlobalString("root")
	}
	if c.GlobalIsSet("runroot") {
		config.RunRoot = c.GlobalString("runroot")
	}

	if c.GlobalIsSet("storage-driver") {
		config.Storage = c.GlobalString("storage-driver")
	}
	if c.GlobalIsSet("storage-opt") {
		opts := c.GlobalStringSlice("storage-opt")
		if len(opts) > 0 {
			config.StorageOptions = opts
		}
	}
	return config, nil
}
