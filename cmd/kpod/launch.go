package main

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/kubernetes-incubator/cri-o/server"
	"github.com/opencontainers/runc/libcontainer/selinux"
	"github.com/urfave/cli"
	"golang.org/x/net/context"
	pb "k8s.io/kubernetes/pkg/kubelet/api/v1alpha1/runtime"
)

// LAUNCH TODO
// Remove container (and sandbox if we created it) - high priority for usability & testing
// Terminal attach implementation (kpod attach command?)
// Logging (interaction with crio daemon?)
// Properly place created containers in cgroups
// Sanely populate metadata for sandbox
// Missing parsing in CLI handling - DNS, port forwards, mounts, devices, resource limits etc
// Labels and Annotations (pod & container)
// Security & confinement - SELinux, AppArmor, seccomp, capabilities, run as users
// Interface with crio daemon - locking to prevent trampling pod status
// Launch containers in existing sandboxes
// Integration tests
// Man pages

var launchCommand = cli.Command{
	Name:  "launch",
	Usage: "launch a pod",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "image",
			Value: "",
			Usage: "image to launch",
		},
		cli.BoolFlag{
			Name:  "attach",
			Usage: "attach to the container once it is created",
		},
		cli.StringSliceFlag{
			Name:  "env",
			Usage: "specify environment variables to be set inside launched container, specified as KEY=VALUE",
		},
		cli.StringFlag{
			Name:  "labels, l",
			Value: "",
			Usage: "specify labels to be set on launched container",
		},
		cli.StringFlag{
			Name:  "limits",
			Value: "",
			Usage: "specify resource limits for launched container",
		},
		cli.StringFlag{
			Name:  "ports",
			Value: "",
			Usage: "specify ports to be forwarded to launched container",
		},
		cli.BoolFlag{
			Name:  "rm",
			Usage: "remove launched container (and pod, if a new pod was created) after it exits",
		},
		cli.BoolFlag{
			Name:  "stdin, i",
			Usage: "keep stdin open on launched container",
		},
		cli.BoolFlag{
			Name:  "tty, t",
			Usage: "allocate a TTY for launched container",
		},
		cli.StringSliceFlag{
			Name:  "mount",
			Usage: "attach mounts on the host to created container",
		},
		cli.StringSliceFlag{
			Name:  "device",
			Usage: "make host devices available inside the container",
		},
		cli.StringSliceFlag{
			Name:  "dns",
			Usage: "set DNS servers for container",
		},
		cli.StringSliceFlag{
			Name:  "dns-search",
			Usage: "set DNS search domains for container",
		},
		cli.StringFlag{
			Name:  "pod",
			Value: "",
			Usage: "launch container inside an existing pod",
		},
		cli.BoolFlag{
			Name:  "privileged",
			Usage: "launch a privileged container",
		},
		cli.BoolFlag{
			Name:  "read-only",
			Usage: "mount root of created container as read only",
		},
		cli.BoolFlag{
			Name:  "host-network",
			Usage: "don't join a network namespace, and use host's network namespace",
		},
		cli.BoolFlag{
			Name:  "host-ipc",
			Usage: "don't join an IPC namespace, and use the host's IPC namespace",
		},
		cli.BoolFlag{
			Name:  "host-pid",
			Usage: "don't join a PID namespace, and use the host's PID namespace",
		},
	},
	Action: func(ctx *cli.Context) error {
		if ctx.GlobalBool("debug") {
			logrus.SetLevel(logrus.DebugLevel)
		}

		// TODO enable selinux support
		selinux.SetDisabled()

		// Parse CLI options
		cliConfig, err := parseLaunchCLI(ctx)
		if err != nil {
			return fmt.Errorf("error parsing CLI arguments: %v", err)
		}

		// TODO consider moving to a different configuration file syntax for kpod
		// For now, use the existing cri-o one for convenience
		config := new(server.Config)
		err = config.FromFile(cliConfig.configPath)
		if err != nil {
			return fmt.Errorf("could not load configuration file %v: %v", cliConfig.configPath, err)
		}

		if _, err := os.Stat(config.Runtime); os.IsNotExist(err) {
			return fmt.Errorf("specified runtime %v does not exist", config.Runtime)
		}

		server, err := server.New(config)
		if err != nil {
			return fmt.Errorf("error creating server: %v", err)
		}
		// Don't bother with a graceful shutdown for now - containers exist beyond the life of the kpod executable
		// We don't want to shut them & related storage down when kpod itself exits
		//defer server.Shutdown()

		logrus.Debugf("Going to create container named %v with image %v", cliConfig.containerName, cliConfig.image)

		var sandboxId string
		var sandboxConfig *pb.PodSandboxConfig

		sandboxSecurityConfig, containerSecurityConfig, err := generateLinuxSecurityConfigs(cliConfig)
		if err != nil {
			return fmt.Errorf("error generating security configuration: %v", err)
		}

		if cliConfig.pod != "" {
			// We were passed a pod to join
			// Don't create our own
			sandboxId = cliConfig.pod

			logrus.Debugf("Joining existing sandbox with ID %v", sandboxId)

			// Get status of pod - verify it exists
			sandboxStatusRequest := pb.PodSandboxStatusRequest{
				PodSandboxId: sandboxId,
			}
			sandboxStatusResp, err := server.PodSandboxStatus(context.Background(), &sandboxStatusRequest)
			if err != nil {
				return fmt.Errorf("error getting status of sandbox %v: %v", sandboxId, err)
			}

			if sandboxStatusResp.Status.State != pb.PodSandboxState_SANDBOX_READY {
				return fmt.Errorf("cannot attach to sandbox %v as it is not ready", sandboxId)
			}

			// TODO - Before this can work, we need a way of getting the PodSandboxConfig from the server
			// As we are not the executable that created the sandbox, we don't have the config to use
			sandboxConfig = nil

			// TODO validity checks - once we get the sandbox's config, we should check given CLI options
			// To check for conflicts between them and server config
		} else {
			logrus.Debugf("Creating new sandbox for container")

			sandboxConfig, err = makePodSandboxConfig(cliConfig, sandboxSecurityConfig)
			if err != nil {
				return fmt.Errorf("error creating PodSandboxConfig: %v", err)
			}

			sandboxRequest := pb.RunPodSandboxRequest{
				Config: sandboxConfig,
			}
			sandboxResp, err := server.RunPodSandbox(context.Background(), &sandboxRequest)
			if err != nil {
				return fmt.Errorf("error creating sandbox: %v", err)
			}
			sandboxId = sandboxResp.PodSandboxId

			logrus.Infof("Successfully created sandbox with ID %v", sandboxId)
		}

		createRequest, err := makeContainerCreateRequest(cliConfig, containerSecurityConfig, sandboxId, sandboxConfig)
		if err != nil {
			return fmt.Errorf("error creating ContainerCreateRequest: %v", err)
		}

		containerResp, err := server.CreateContainer(context.Background(), createRequest)
		if err != nil {
			if cliConfig.pod != "" {
				// If we did not join an existing pod, clean up the sandbox we made
				defer func() {
					removeRequest := pb.RemovePodSandboxRequest{
						PodSandboxId: sandboxId,
					}
					_, err = server.RemovePodSandbox(context.Background(), &removeRequest)
					if err != nil {
						logrus.Fatalf("Failed to remove sandbox %v: %v", sandboxId, err)
					}
				}()
			}

			return fmt.Errorf("error creating container: %v", err)
		}

		containerId := containerResp.ContainerId

		logrus.Debugf("Successfully created container with ID %v in sandbox %v", sandboxId, containerId)

		startRequest := pb.StartContainerRequest{
			ContainerId: containerId,
		}

		// StartContainerResponse is an empty struct, just ignore it
		_, err = server.StartContainer(context.Background(), &startRequest)
		if err != nil {
			return fmt.Errorf("error starting container: %v", err)
		}

		logrus.Infof("Successfully started container with ID %v", containerId)

		return nil
	},
}

// TODO add:
// - seccomp
// - apparmor
// - capabilities
// - selinux
// - annotations
// - logging configuration
// - cgroup config
// - Maybe working directory of container?
type launchConfig struct {
	configPath    string
	containerName string
	command       string
	args          *[]string
	image         string
	attach        bool
	env           []*pb.KeyValue
	labels        *map[string]string // TODO - right now we set same labels for both pod and container. Worth splitting?
	limits        *pb.LinuxContainerResources
	ports         []*pb.PortMapping
	remove        bool
	stdin         bool
	stdinOnce     bool
	tty           bool
	mount         []*pb.Mount
	devices       []*pb.Device
	dns           *pb.DNSConfig
	pod           string
	privileged    bool
	readOnlyRoot  bool
	hostNet       bool
	hostIpc       bool
	hostPid       bool
}

func parseLaunchCLI(ctx *cli.Context) (*launchConfig, error) {
	config := new(launchConfig)

	// Parse required flags

	if !ctx.IsSet("image") {
		return nil, fmt.Errorf("must provide image to launch")
	}
	config.image = ctx.String("image")

	// Parse required arguments

	if ctx.NArg() == 0 {
		return nil, fmt.Errorf("must provide name of container")
	}
	config.containerName = ctx.Args().First()

	// TODO: currently it is very easy to confuse name and command
	// Requiring a -- to separate the two (akin to kubectl run) could improve matters

	// Parse optional arguments

	if ctx.NArg() > 1 {
		// First additional arg after name is command
		config.command = ctx.Args().Get(1)
		// Any others after that are ordered args
		if ctx.NArg() > 2 {
			args := make([]string, ctx.NArg()-2)
			for i := 2; i < ctx.NArg(); i++ {
				args[i-2] = ctx.Args().Get(i)
			}
			config.args = &args
		}
	}

	// Parse optional flags

	config.configPath = ctx.GlobalString("config")

	// TODO remove when terminal forwarding is finished
	if ctx.IsSet("attach") {
		return nil, fmt.Errorf("attach functionality is not yet implemented")
	}
	config.attach = ctx.Bool("attach")

	if ctx.IsSet("env") {
		env := ctx.StringSlice("env")
		config.env = make([]*pb.KeyValue, len(env))

		for i := 0; i < len(env); i++ {
			splitResult := strings.Split(env[i], "=")
			if len(splitResult) != 2 {
				return nil, fmt.Errorf("environment must be formatted KEY=VALUE, instead got %v", env[i])
			}

			keyValue := pb.KeyValue{
				Key:   splitResult[0],
				Value: splitResult[1],
			}
			config.env[i] = &keyValue
		}
	}

	if ctx.IsSet("labels") {
		// TODO label parsing code
		return nil, fmt.Errorf("label parsing is not yet implemented")
	} else {
		config.labels = new(map[string]string)
	}
	// TODO should set some default label values - indicate we were launched by kpod, maybe?

	if ctx.IsSet("limits") {
		// TODO resource limit parsing code
		// Should be relatively straightforward aside from units
		return nil, fmt.Errorf("resource limit parsing is not yet implemented")
	}

	if ctx.IsSet("ports") {
		// TODO port mapping parsing
		// Biggest difficulty: probably don't want to force users to specify host IP and protocol
		// Protocol is easy - just create identical rules for TCP and UDP
		// Host IP, maybe not so much, experimentation required to verify
		return nil, fmt.Errorf("port mapping parsing is not yet implemented")
	}

	// TODO remove when container remove is finished
	if ctx.IsSet("rm") {
		return nil, fmt.Errorf("remove functionality is not yet implemented")
	}
	config.remove = ctx.Bool("rm")

	if ctx.IsSet("stdin") {
		config.stdin = true
		config.stdinOnce = false
	}

	config.tty = ctx.Bool("tty")

	if ctx.IsSet("mount") {
		// TODO mount parsing
		return nil, fmt.Errorf("mount parsing is not yet implemented")
	}

	if ctx.IsSet("device") {
		// TODO device parsing
		return nil, fmt.Errorf("device parsing is not yet implemented")
	}

	if ctx.IsSet("dns") || ctx.IsSet("dns-search") {
		// TODO dns parsing
		return nil, fmt.Errorf("DNS parsing is not yet implemented")
	}

	config.privileged = ctx.Bool("privileged")

	config.readOnlyRoot = ctx.Bool("read-only")

	config.hostNet = ctx.Bool("host-net")
	config.hostIpc = ctx.Bool("host-ipc")
	config.hostPid = ctx.Bool("host-pid")

	if ctx.IsSet("pod") {
		// TODO implement joining existing pods
		// Needs modifications to server code to support
		return nil, fmt.Errorf("joining an existing pod is not yet implemented")
	}

	return config, nil
}

func makePodSandboxConfig(cliConfig *launchConfig, securityConfig *pb.LinuxSandboxSecurityContext) (*pb.PodSandboxConfig, error) {
	sandboxId, err := getRandomId()
	if err != nil {
		return nil, fmt.Errorf("error generating sandbox id: %v", err)
	}

	metadata := pb.PodSandboxMetadata{
		Name:      "kpod_launch_" + sandboxId,
		Uid:       "kpod_default",
		Namespace: "kpod_default",
		Attempt:   0,
	}

	// TODO cgroup config
	linuxSandboxConfig := pb.LinuxPodSandboxConfig{
		CgroupParent:    "",
		SecurityContext: securityConfig,
	}

	config := pb.PodSandboxConfig{
		Metadata: &metadata,
		Hostname: "kpod_launch_" + sandboxId,
		// TODO Logging
		LogDirectory: "",
		DnsConfig:    cliConfig.dns,
		PortMappings: cliConfig.ports,
		Labels:       *cliConfig.labels,
		// TODO appropriate annotations
		Annotations: make(map[string]string),
		// TODO populate cgroup and security configuration
		Linux: &linuxSandboxConfig,
	}

	return &config, nil
}

func makeContainerCreateRequest(cliConfig *launchConfig, securityConfig *pb.LinuxContainerSecurityContext, sandboxId string, sandboxConfig *pb.PodSandboxConfig) (*pb.CreateContainerRequest, error) {
	metadata := pb.ContainerMetadata{
		Name:    cliConfig.containerName,
		Attempt: 0,
	}

	linuxConfig := pb.LinuxContainerConfig{
		Resources:       cliConfig.limits,
		SecurityContext: securityConfig,
	}

	config := pb.ContainerConfig{
		Metadata: &metadata,
		Image:    &pb.ImageSpec{Image: cliConfig.image},
		Command:  []string{cliConfig.command},
		Args:     []string{},
		// TODO: allow passing this via CLI (if worthwhile)
		WorkingDir: "",
		Envs:       cliConfig.env,
		Mounts:     cliConfig.mount,
		Devices:    cliConfig.devices,
		Labels:     make(map[string]string),
		// TODO allow setting annotations
		Annotations: nil,
		// TODO Logging
		LogPath:   "",
		Stdin:     cliConfig.stdin,
		StdinOnce: cliConfig.stdinOnce,
		Tty:       cliConfig.tty,
		Linux:     &linuxConfig,
	}

	if cliConfig.args != nil {
		config.Args = *cliConfig.args
	}

	if cliConfig.labels != nil {
		config.Labels = *cliConfig.labels
	}

	req := pb.CreateContainerRequest{
		PodSandboxId:  sandboxId,
		Config:        &config,
		SandboxConfig: sandboxConfig,
	}

	return &req, nil
}

// TODO: Capabilities, SELinux, set non-root user, add additional groups
func generateLinuxSecurityConfigs(cliConfig *launchConfig) (*pb.LinuxSandboxSecurityContext, *pb.LinuxContainerSecurityContext, error) {
	linuxNamespaceOption := pb.NamespaceOption{
		HostNetwork: cliConfig.hostNet,
		HostPid:     cliConfig.hostPid,
		HostIpc:     cliConfig.hostIpc,
	}

	// Just run as root for now
	runAsUser := pb.Int64Value{
		Value: 0,
	}

	sandboxConfig := pb.LinuxSandboxSecurityContext{
		NamespaceOptions:   &linuxNamespaceOption,
		SelinuxOptions:     nil,
		RunAsUser:          &runAsUser, // Probably not strictly necessary to set this here if we're doing it below
		ReadonlyRootfs:     cliConfig.readOnlyRoot,
		SupplementalGroups: []int64{},
		Privileged:         cliConfig.privileged,
	}

	containerConfig := pb.LinuxContainerSecurityContext{
		Capabilities:       &pb.Capability{},
		Privileged:         cliConfig.privileged,
		NamespaceOptions:   &linuxNamespaceOption,
		SelinuxOptions:     nil,
		RunAsUser:          &runAsUser,
		RunAsUsername:      "",
		ReadonlyRootfs:     cliConfig.readOnlyRoot,
		SupplementalGroups: []int64{},
	}

	return &sandboxConfig, &containerConfig, nil
}

// Generate and hex-encode 128-bit random ID
func getRandomId() (string, error) {
	urandom, err := os.Open("/dev/urandom")
	if err != nil {
		return "", fmt.Errorf("could not open urandom for reading: %v", err)
	}

	defer urandom.Close()

	data := make([]byte, 16)
	count, err := urandom.Read(data)
	if err != nil {
		return "", fmt.Errorf("error reading from urandom: %v", err)
	} else if count != 16 {
		return "", fmt.Errorf("read too few bytes from urandom")
	}

	return hex.EncodeToString(data), nil
}
