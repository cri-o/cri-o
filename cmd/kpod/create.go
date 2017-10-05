package main

import (
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	pb "k8s.io/kubernetes/pkg/kubelet/apis/cri/v1alpha1/runtime"
)

// TODO: Missing flags from docker-create - particularly --security-opt or equiv and resource limit flags
// TODO stop using Kubernetes API structs here - replace with our own versions (remove pb import entirely)
// TODO Add missing flags from docker-create
// TODO parse flags into a createConfig, and parse createConfig into an OCI runtime spec

// TODO These temporary values should be replaced with sane defaults
const (
	defaultHostname     = "kpod-launch"
	defaultCgroupParent = "/kpod-launch"
)

type createConfig struct {
	image            string
	command          string
	args             []string
	pod              string
	privileged       bool
	rm               bool
	hostNet          bool
	hostPID          bool
	hostIPC          bool
	name             string
	labels           map[string]string
	workDir          string
	env              map[string]string
	detach           bool
	stdin            bool
	tty              bool
	devices          []*pb.Device
	mounts           []*pb.Mount
	capAdd           []string
	capDrop          []string
	dnsServers       []string
	dnsSearch        []string
	dnsOpt           []string
	ports            []*pb.PortMapping
	hostname         string
	cgroupParent     string
	sysctl           string
	user             int64
	additionalGroups []int64
	readOnlyRootfs   bool
}

var createFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "pod",
		Usage: "Run container in an existing pod",
	},
	cli.BoolFlag{
		Name:  "privileged",
		Usage: "Run a privileged container",
	},
	cli.BoolFlag{
		Name:  "rm",
		Usage: "Remove container (and pod if created) after exit",
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
		Name:  "ipc",
		Usage: "Use `host` IPC namespace",
	},
	cli.StringFlag{
		Name:  "name",
		Usage: "Assign a name to the container",
	},
	cli.StringSliceFlag{
		Name:  "label",
		Usage: "Set label metadata on container",
	},
	cli.StringFlag{
		Name:  "workdir, w",
		Usage: "Set working `directory` of container",
		Value: "/",
	},
	cli.StringSliceFlag{
		Name:  "env, e",
		Usage: "Set environment variables in container",
	},
	cli.BoolFlag{
		Name:  "detach, d",
		Usage: "Start container detached",
	},
	cli.BoolFlag{
		Name:  "interactive, i",
		Usage: "Keep STDIN open even if deatched",
	},
	cli.BoolFlag{
		Name:  "tty, t",
		Usage: "Allocate a TTY for container",
	},
	cli.StringSliceFlag{
		Name:  "device",
		Usage: "Mount devices into the container",
	},
	cli.StringSliceFlag{
		Name:  "volume, v",
		Usage: "Mount volumes into the container",
	},
	cli.StringSliceFlag{
		Name:  "cap-add",
		Usage: "Add capabilities to the container",
	},
	cli.StringSliceFlag{
		Name:  "cap-drop",
		Usage: "Drop capabilities from the container",
	},
	cli.StringSliceFlag{
		Name:  "dns",
		Usage: "Set custom DNS servers",
	},
	cli.StringSliceFlag{
		Name:  "dns-search",
		Usage: "Set custom DNS search domains",
	},
	cli.StringSliceFlag{
		Name:  "dns-opt",
		Usage: "Set custom DNS options",
	},
	cli.StringSliceFlag{
		Name:  "expose",
		Usage: "Expose a port",
	},
	cli.StringFlag{
		Name:  "hostname, h",
		Usage: "Set hostname",
		Value: defaultHostname,
	},
	cli.StringFlag{
		Name:  "cgroup-parent",
		Usage: "Set CGroup parent",
		Value: defaultCgroupParent,
	},
	cli.StringFlag{
		Name:  "sysctl",
		Usage: "Set namespaced SYSCTLs",
	},
	cli.StringFlag{
		Name:  "user, u",
		Usage: "Specify user to run as",
	},
	cli.StringFlag{
		Name:  "group-add",
		Usage: "Specify additional groups to run as",
	},
	cli.BoolFlag{
		Name:  "read-only",
		Usage: "Make root filesystem read-only",
	},
}

var createCommand = cli.Command{
	Name:        "create",
	Usage:       "create but do not start a container",
	Description: `Create a new container from specified image, but do not start it`,
	Flags:       createFlags,
	Action:      createCmd,
	ArgsUsage:   "IMAGE [COMMAND [ARG...]]",
}

func createCmd(c *cli.Context) error {
	// TODO should allow user to create based off a directory on the host not just image
	// Need CLI support for this
	if len(c.Args()) != 1 {
		return errors.Errorf("must specify name of image to create from")
	}
	if err := validateFlags(c, createFlags); err != nil {
		return err
	}
	runtime, err := getRuntime(c)
	if err != nil {
		return errors.Wrapf(err, "error creating libpod runtime")
	}
	_ = runtime.GetConfig()

	return errors.Errorf("NOT IMPLEMENTED")
}
