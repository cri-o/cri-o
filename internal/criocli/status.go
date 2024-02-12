package criocli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cri-o/cri-o/internal/client"

	"github.com/urfave/cli/v2"
)

const (
	defaultSocket = "/var/run/crio/crio.sock"
	idArg         = "id"
	socketArg     = "socket"
	jsonArg       = "json"
)

var StatusCommand = &cli.Command{
	Name:  "status",
	Usage: "Display status information",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:      socketArg,
			Aliases:   []string{"s"},
			Usage:     "absolute path to the unix socket",
			Value:     defaultSocket,
			TakesFile: true,
		},
		&cli.BoolFlag{
			Name:    jsonArg,
			Aliases: []string{"j"},
			Usage:   "output JSON instead of plain text",
		},
	},
	OnUsageError: func(c *cli.Context, e error, b bool) error { return e },
	Subcommands: []*cli.Command{{
		Action:  configSubCommand,
		Aliases: []string{"c"},
		Name:    "config",
		Usage:   "Show the configuration of CRI-O as a TOML string.",
	}, {
		Action:  containers,
		Aliases: []string{"container", "cs", "s"},
		Flags: []cli.Flag{&cli.StringFlag{
			Name:    idArg,
			Aliases: []string{"i"},
			Usage:   "the container ID",
		}},
		Name:  "containers",
		Usage: "Display detailed information about the provided container ID.",
	}, {
		Action:  info,
		Aliases: []string{"i"},
		Name:    "info",
		Usage:   "Retrieve generic information about CRI-O, such as the cgroup and storage driver.",
	}},
}

func configSubCommand(c *cli.Context) error {
	crioClient, err := crioClient(c)
	if err != nil {
		return err
	}

	info, err := crioClient.ConfigInfo()
	if err != nil {
		return err
	}

	fmt.Print(info)
	return nil
}

func containers(c *cli.Context) error {
	crioClient, err := crioClient(c)
	if err != nil {
		return err
	}

	id := c.String(idArg)
	if id == "" {
		return fmt.Errorf("the argument --%s cannot be empty", idArg)
	}

	info, err := crioClient.ContainerInfo(c.String(idArg))
	if err != nil {
		return err
	}

	if c.IsSet(jsonArg) {
		j, err := json.Marshal(info)
		if err != nil {
			return fmt.Errorf("marshal JSON: %w", err)
		}
		fmt.Print(string(j))
		return nil
	}

	fmt.Printf("name: %s\n", info.Name)
	fmt.Printf("pid: %d\n", info.Pid)
	fmt.Printf("image: %s\n", info.Image)
	fmt.Printf("image ref: %s\n", info.ImageRef)

	if len(info.ImageDigests) > 0 {
		fmt.Print("image digests:\n")
		for _, v := range info.ImageDigests {
			fmt.Printf("  %s\n", v)
		}
	}

	fmt.Printf("created: %v\n", info.CreatedTime)
	fmt.Printf("labels:\n")
	for k, v := range info.Labels {
		fmt.Printf("  %s: %s\n", k, v)
	}
	fmt.Printf("annotations:\n")
	for k, v := range info.Annotations {
		fmt.Printf("  %s: %s\n", k, v)
	}
	fmt.Printf("CRI-O annotations:\n")
	for k, v := range info.CrioAnnotations {
		fmt.Printf("  %s: %s\n", k, v)
	}
	fmt.Printf("log path: %s\n", info.LogPath)
	fmt.Printf("graph root: %s\n", info.Root)
	fmt.Printf("sandbox: %s\n", info.Sandbox)
	fmt.Printf("ips: %s\n", strings.Join(info.IPs, ", "))

	return nil
}

func info(c *cli.Context) error {
	crioClient, err := crioClient(c)
	if err != nil {
		return err
	}

	info, err := crioClient.DaemonInfo()
	if err != nil {
		return err
	}

	if c.IsSet(jsonArg) {
		j, err := json.Marshal(info)
		if err != nil {
			return fmt.Errorf("marshal JSON: %w", err)
		}
		fmt.Print(string(j))
		return nil
	}

	fmt.Printf("cgroup driver: %s\n", info.CgroupDriver)
	fmt.Printf("storage driver: %s\n", info.StorageDriver)
	fmt.Printf("storage graph root: %s\n", info.StorageRoot)
	fmt.Printf("storage image: %s\n", info.StorageImage)

	fmt.Printf("default GID mappings (format <container>:<host>:<size>):\n")
	for _, m := range info.DefaultIDMappings.Gids {
		fmt.Printf("  %d:%d:%d\n", m.ContainerID, m.HostID, m.Size)
	}
	fmt.Printf("default UID mappings (format <container>:<host>:<size>):\n")
	for _, m := range info.DefaultIDMappings.Uids {
		fmt.Printf("  %d:%d:%d\n", m.ContainerID, m.HostID, m.Size)
	}

	return nil
}

func crioClient(c *cli.Context) (client.CrioClient, error) {
	return client.New(c.String(socketArg))
}
