package main

import (
	"os"
	"reflect"
	"regexp"
	"strings"

	is "github.com/containers/image/storage"
	"github.com/containers/storage"
	"github.com/fatih/camelcase"
	"github.com/kubernetes-incubator/cri-o/libkpod"
	"github.com/kubernetes-incubator/cri-o/libpod"
	"github.com/kubernetes-incubator/cri-o/server"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

var (
	stores = make(map[storage.Store]struct{})
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
	stores[store] = struct{}{}
	return store, nil
}

func getRuntime(c *cli.Context) (*libpod.Runtime, error) {

	config, err := getConfig(c)
	if err != nil {
		return nil, errors.Wrapf(err, "could not get config")
	}

	options := storage.DefaultStoreOptions
	options.GraphRoot = config.Root
	options.RunRoot = config.RunRoot
	options.GraphDriverName = config.Storage
	options.GraphDriverOptions = config.StorageOptions

	return libpod.NewRuntime(libpod.WithStorageConfig(options))
}

func shutdownStores() {
	for store := range stores {
		if _, err := store.Shutdown(false); err != nil {
			break
		}
	}
}

func getConfig(c *cli.Context) (*libkpod.Config, error) {
	config := libkpod.DefaultConfig()
	var configFile string
	if c.GlobalIsSet("config") {
		configFile = c.GlobalString("config")
	} else if _, err := os.Stat(server.CrioConfigPath); err == nil {
		configFile = server.CrioConfigPath
	}
	// load and merge the configfile from the commandline or use
	// the default crio config file
	if configFile != "" {
		err := config.UpdateFromFile(configFile)
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
	if c.GlobalIsSet("runtime") {
		config.Runtime = c.GlobalString("runtime")
	}
	return config, nil
}

func splitCamelCase(src string) string {
	entries := camelcase.Split(src)
	return strings.Join(entries, " ")
}

// validateFlags searches for StringFlags or StringSlice flags that never had
// a value set.  This commonly occurs when the CLI mistakenly takes the next
// option and uses it as a value.
func validateFlags(c *cli.Context, flags []cli.Flag) error {
	for _, flag := range flags {
		switch reflect.TypeOf(flag).String() {
		case "cli.StringSliceFlag":
			{
				f := flag.(cli.StringSliceFlag)
				name := strings.Split(f.Name, ",")
				val := c.StringSlice(name[0])
				for _, v := range val {
					if ok, _ := regexp.MatchString("^-.+", v); ok {
						return errors.Errorf("option --%s requires a value", name[0])
					}
				}
			}
		case "cli.StringFlag":
			{
				f := flag.(cli.StringFlag)
				name := strings.Split(f.Name, ",")
				val := c.String(name[0])
				if ok, _ := regexp.MatchString("^-.+", val); ok {
					return errors.Errorf("option --%s requires a value", name[0])
				}
			}
		}
	}
	return nil
}

// Common flags shared between commands
var createFlags = []cli.Flag{
	cli.StringSliceFlag{
		Name:  "cap-add",
		Usage: "Add capabilities to the container",
	},
	cli.StringSliceFlag{
		Name:  "cap-drop",
		Usage: "Drop capabilities from the container",
	},
	cli.StringFlag{
		Name:  "cgroup-parent",
		Usage: "Set CGroup parent",
		Value: defaultCgroupParent,
	},
	cli.BoolFlag{
		Name:  "detach, d",
		Usage: "Start container detached",
	},
	cli.StringSliceFlag{
		Name:  "device",
		Usage: "Mount devices into the container",
	},
	cli.StringSliceFlag{
		Name:  "dns",
		Usage: "Set custom DNS servers",
	},
	cli.StringSliceFlag{
		Name:  "dns-opt",
		Usage: "Set custom DNS options",
	},
	cli.StringSliceFlag{
		Name:  "dns-search",
		Usage: "Set custom DNS search domains",
	},
	cli.StringSliceFlag{
		Name:  "env, e",
		Usage: "Set environment variables in container",
	},
	cli.StringSliceFlag{
		Name:  "expose",
		Usage: "Expose a port",
	},
	cli.StringFlag{
		Name:  "group-add",
		Usage: "Specify additional groups to run as",
	},
	cli.StringFlag{
		Name:  "hostname, h",
		Usage: "Set hostname",
		Value: defaultHostname,
	},
	cli.BoolFlag{
		Name:  "interactive, i",
		Usage: "Keep STDIN open even if deatched",
	},
	cli.StringFlag{
		Name:  "ipc",
		Usage: "Use `host` IPC namespace",
	},
	cli.StringSliceFlag{
		Name:  "label",
		Usage: "Set label metadata on container",
	},
	cli.StringFlag{
		Name:  "name",
		Usage: "Assign a name to the container",
	},
	cli.StringFlag{
		Name:  "network",
		Usage: "Use `host` network namespace",
	},
	cli.StringFlag{
		Name:  "pid",
		Usage: "Use `host` PID namespace",
	},
	cli.StringFlag{
		Name:  "pod",
		Usage: "Run container in an existing pod",
	},
	cli.BoolFlag{
		Name:  "privileged",
		Usage: "Run a privileged container",
	},
	cli.BoolFlag{
		Name:  "read-only",
		Usage: "Make root filesystem read-only",
	},
	cli.BoolFlag{
		Name:  "rm",
		Usage: "Remove container (and pod if created) after exit",
	},
	cli.StringFlag{
		Name:  "sysctl",
		Usage: "Set namespaced SYSCTLs",
	},
	cli.BoolFlag{
		Name:  "tty, t",
		Usage: "Allocate a TTY for container",
	},
	cli.StringFlag{
		Name:  "user, u",
		Usage: "Specify user to run as",
	},

	cli.StringSliceFlag{
		Name:  "volume, v",
		Usage: "Mount volumes into the container",
	},
	cli.StringFlag{
		Name:  "workdir, w",
		Usage: "Set working `directory` of container",
		Value: "/",
	},
}
