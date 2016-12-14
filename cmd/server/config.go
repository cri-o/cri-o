package main

import (
	"os"
	"path/filepath"
	"text/template"

	"github.com/kubernetes-incubator/cri-o/manager"
	"github.com/opencontainers/runc/libcontainer/selinux"
	"github.com/urfave/cli"
)

const (
	ocidRoot            = "/var/lib/ocid"
	conmonPath          = "/usr/libexec/ocid/conmon"
	pausePath           = "/usr/libexec/ocid/pause"
	seccompProfilePath  = "/etc/ocid/seccomp.json"
	apparmorProfileName = "ocid-default"
)

var commentedConfigTemplate = template.Must(template.New("config").Parse(`
# The "ocid" table contains all of the server options.
[ocid]

# root is a path to the "root directory". OCID stores all of its state
# data, including container images, in this directory.
root = "{{ .Root }}"

# sandbox_dir is the directory where ocid will store all of its sandbox
# state and other information.
sandbox_dir = "{{ .SandboxDir }}"

# container_dir is the directory where ocid will store all of its
# container state and other information.
container_dir = "{{ .ContainerDir }}"

# The "ocid.api" table contains settings for the kubelet/gRPC
# interface (which is also used by ocic).
[ocid.api]

# listen is the path to the AF_LOCAL socket on which ocid will listen.
listen = "{{ .Listen }}"

# The "ocid.runtime" table contains settings pertaining to the OCI
# runtime used and options for how to set up and manage the OCI runtime.
[ocid.runtime]

# runtime is a path to the OCI runtime which ocid will be using.
runtime = "{{ .Runtime }}"

# conmon is the path to conmon binary, used for managing the runtime.
conmon = "{{ .Conmon }}"

# conmon_env is the environment variable list for conmon process,
# used for passing necessary environment variable to conmon or runtime.
conmon_env = [
{{ range $env := .ConmonEnv }}{{ printf "\t%q,\n" $env }}{{ end }}]

# selinux indicates whether or not SELinux will be used for pod
# separation on the host. If you enable this flag, SELinux must be running
# on the host.
selinux = {{ .SELinux }}

# seccomp_profile is the seccomp json profile path which is used as the
# default for the runtime.
seccomp_profile = "{{ .SeccompProfile }}"

# apparmor_profile is the apparmor profile name which is used as the
# default for the runtime.
apparmor_profile = "{{ .ApparmorProfile }}"

# The "ocid.image" table contains settings pertaining to the
# management of OCI images.
[ocid.image]

# pause is the path to the statically linked pause container binary, used
# as the entrypoint for infra containers.
pause = "{{ .Pause }}"
`))

// TODO: Currently ImageDir isn't really used, so we haven't added it to this
//       template. Add it once the storage code has been merged.

// DefaultConfig returns the default configuration for ocid.
func DefaultConfig() *manager.Config {
	return &manager.Config{
		RootConfig: manager.RootConfig{
			Root:         ocidRoot,
			SandboxDir:   filepath.Join(ocidRoot, "sandboxes"),
			ContainerDir: filepath.Join(ocidRoot, "containers"),
			LogDir:       "/var/log/ocid/pods",
		},
		APIConfig: manager.APIConfig{
			Listen: "/var/run/ocid.sock",
		},
		RuntimeConfig: manager.RuntimeConfig{
			Runtime: "/usr/bin/runc",
			Conmon:  conmonPath,
			ConmonEnv: []string{
				"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			},
			SELinux:         selinux.SelinuxEnabled(),
			SeccompProfile:  seccompProfilePath,
			ApparmorProfile: apparmorProfileName,
		},
		ImageConfig: manager.ImageConfig{
			Pause:    pausePath,
			ImageDir: filepath.Join(ocidRoot, "store"),
		},
	}
}

var configCommand = cli.Command{
	Name:  "config",
	Usage: "generate ocid configuration files",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "default",
			Usage: "output the default configuration",
		},
	},
	Action: func(c *cli.Context) error {
		// At this point, app.Before has already parsed the user's chosen
		// config file. So no need to handle that here.
		config := c.App.Metadata["config"].(*manager.Config)
		if c.Bool("default") {
			config = DefaultConfig()
		}

		// Output the commented config.
		return commentedConfigTemplate.ExecuteTemplate(os.Stdout, "config", config)
	},
}
