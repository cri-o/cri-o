package main

import (
	"os"
	"text/template"

	"github.com/cri-o/cri-o/server"
	"github.com/urfave/cli"
)

// NOTE: please propagate any changes to the template to docs/crio.conf.5.md

var commentedConfigTemplate = template.Must(template.New("config").Parse(`
# The CRI-O configuration file specifies all of the available configuration
# options and command-line flags for the crio(8) OCI Kubernetes Container Runtime
# daemon, but in a TOML format that can be more easily modified and versioned.
#
# Please refer to crio.conf(5) for details of all configuration options.

# CRI-O reads its storage defaults from the containers-storage.conf(5) file
# located at /etc/containers/storage.conf. Modify this storage configuration if
# you want to change the system's defaults. If you want to modify storage just
# for CRI-O, you can change the storage configuration options here.
[crio]

# Path to the "root directory". CRI-O stores all of its data, including
# containers images, in this directory.
#root = "{{ .Root }}"

# Path to the "run directory". CRI-O stores all of its state in this directory.
#runroot = "{{ .RunRoot }}"

# Storage driver used to manage the storage of images and containers. Please
# refer to containers-storage.conf(5) to see all available storage drivers.
#storage_driver = "{{ .Storage }}"

# List to pass options to the storage driver. Please refer to
# containers-storage.conf(5) to see all available storage options.
#storage_option = [
{{ range $opt := .StorageOptions }}{{ printf "#\t%q,\n" $opt }}{{ end }}#]

# Location for CRI-O to place the version file, and for crio-wipe to look for the
# version file
version_file_location = "{{ .VersionFileLocation }}"

# If set to false, in-memory locking will be used instead of file-based locking.
file_locking = {{ .FileLocking }}

# Path to the lock file.
file_locking_path = "{{ .FileLockingPath }}"

# The crio.api table contains settings for the kubelet/gRPC interface.
[crio.api]

# Path to AF_LOCAL socket on which CRI-O will listen.
listen = "{{ .Listen }}"

# IP address on which the stream server will listen.
stream_address = "{{ .StreamAddress }}"

# The port on which the stream server will listen.
stream_port = "{{ .StreamPort }}"

# Enable encrypted TLS transport of the stream server.
stream_enable_tls = {{ .StreamEnableTLS }}

# Path to the x509 certificate file used to serve the encrypted stream. This
# file can change, and CRI-O will automatically pick up the changes within 5
# minutes.
stream_tls_cert = "{{ .StreamTLSCert }}"

# Path to the key file used to serve the encrypted stream. This file can
# change, and CRI-O will automatically pick up the changes within 5 minutes.
stream_tls_key = "{{ .StreamTLSKey }}"

# Path to the x509 CA(s) file used to verify and authenticate client
# communication with the encrypted stream. This file can change, and CRI-O will
# automatically pick up the changes within 5 minutes.
stream_tls_ca = "{{ .StreamTLSCA }}"

# Maximum grpc send message size in bytes. If not set or <=0, then CRI-O will default to 16 * 1024 * 1024.
grpc_max_send_msg_size = {{ .GRPCMaxSendMsgSize }}

# Maximum grpc receive message size. If not set or <= 0, then CRI-O will default to 16 * 1024 * 1024.
grpc_max_recv_msg_size = {{ .GRPCMaxRecvMsgSize }}

# The crio.runtime table contains settings pertaining to the OCI runtime used
# and options for how to set up and manage the OCI runtime.
[crio.runtime]

# A list of ulimits to be set in containers by default, specified as
# "<ulimit name>=<soft limit>:<hard limit>", for example:
# "nofile=1024:2048"
# If nothing is set here, settings will be inherited from the CRI-O daemon
#default_ulimits = [
{{ range $ulimit := .DefaultUlimits }}{{ printf "#\t%q,\n" $ulimit }}{{ end }}#]

# default_runtime is the _name_ of the OCI runtime to be used as the default.
# The name is matched against the runtimes map below.
default_runtime = "{{ .DefaultRuntime }}"

# If true, the runtime will not use pivot_root, but instead use MS_MOVE.
no_pivot = {{ .NoPivot }}

# Path to the conmon binary, used for monitoring the OCI runtime.
conmon = "{{ .Conmon }}"

# Environment variable list for the conmon process, used for passing necessary
# environment variables to conmon or the runtime.
conmon_env = [
{{ range $env := .ConmonEnv }}{{ printf "\t%q,\n" $env }}{{ end }}]

# If true, SELinux will be used for pod separation on the host.
selinux = {{ .SELinux }}

# Path to the seccomp.json profile which is used as the default seccomp profile
# for the runtime.
seccomp_profile = "{{ .SeccompProfile }}"

# Used to change the name of the default AppArmor profile of CRI-O. The default
# profile name is "crio-default-" followed by the version string of CRI-O.
apparmor_profile = "{{ .ApparmorProfile }}"

# Cgroup management implementation used for the runtime.
cgroup_manager = "{{ .CgroupManager }}"

# List of default capabilities for containers. If it is empty or commented out,
# only the capabilities defined in the containers json file by the user/kube
# will be added.
default_capabilities = [
{{ range $capability := .DefaultCapabilities}}{{ printf "\t%q, \n" $capability}}{{ end }}]

# List of default sysctls. If it is empty or commented out, only the sysctls
# defined in the container json file by the user/kube will be added.
default_sysctls = [
{{ range $sysctl := .DefaultSysctls}}{{ printf "\t%q, \n" $sysctl}}{{ end }}]

# List of additional devices. specified as
# "<device-on-host>:<device-on-container>:<permissions>", for example: "--device=/dev/sdc:/dev/xvdc:rwm".
#If it is empty or commented out, only the devices
# defined in the container json file by the user/kube will be added.
additional_devices = [
{{ range $device := .AdditionalDevices}}{{ printf "\t%q, \n" $device}}{{ end }}]

# Path to OCI hooks directories for automatically executed hooks.
hooks_dir = [
{{ range $hooksDir := .HooksDir }}{{ printf "\t%q, \n" $hooksDir}}{{ end }}]

# List of default mounts for each container. **Deprecated:** this option will
# be removed in future versions in favor of default_mounts_file.
default_mounts = [
{{ range $mount := .DefaultMounts }}{{ printf "\t%q, \n" $mount }}{{ end }}]

# Path to the file specifying the defaults mounts for each container. The
# format of the config is /SRC:/DST, one mount per line. Notice that CRI-O reads
# its default mounts from the following two files:
#
#   1) /etc/containers/mounts.conf (i.e., default_mounts_file): This is the
#      override file, where users can either add in their own default mounts, or
#      override the default mounts shipped with the package.
#
#   2) /usr/share/containers/mounts.conf: This is the default file read for
#      mounts. If you want CRI-O to read from a different, specific mounts file,
#      you can change the default_mounts_file. Note, if this is done, CRI-O will
#      only add mounts it finds in this file.
#
#default_mounts_file = "{{ .DefaultMountsFile }}"

# Maximum number of processes allowed in a container.
pids_limit = {{ .PidsLimit }}

# Maximum sized allowed for the container log file. Negative numbers indicate
# that no size limit is imposed. If it is positive, it must be >= 8192 to
# match/exceed conmon's read buffer. The file is truncated and re-opened so the
# limit is never exceeded.
log_size_max = {{ .LogSizeMax }}

# Whether container output should be logged to journald in addition to the kuberentes log file
log_to_journald = {{ .LogToJournald }}

# Path to directory in which container exit files are written to by conmon.
container_exits_dir = "{{ .ContainerExitsDir }}"

# Path to directory for container attach sockets.
container_attach_socket_dir = "{{ .ContainerAttachSocketDir }}"

# If set to true, all containers will run in read-only mode.
read_only = {{ .ReadOnly }}

# Changes the verbosity of the logs based on the level it is set to. Options
# are fatal, panic, error, warn, info, and debug.
log_level = "{{ .LogLevel }}"

# The UID mappings for the user namespace of each container. A range is
# specified in the form containerUID:HostUID:Size. Multiple ranges must be
# separated by comma.
uid_mappings = "{{ .UIDMappings }}"

# The GID mappings for the user namespace of each container. A range is
# specified in the form containerGID:HostGID:Size. Multiple ranges must be
# separated by comma.
gid_mappings = "{{ .GIDMappings }}"

# The minimal amount of time in seconds to wait before issuing a timeout
# regarding the proper termination of the container.
ctr_stop_timeout = {{ .CtrStopTimeout }}

  # The "crio.runtime.runtimes" table defines a list of OCI compatible runtimes.
  # The runtime to use is picked based on the runtime_handler provided by the CRI.
  # If no runtime_handler is provided, the runtime will be picked based on the level
  # of trust of the workload.
  {{ range $runtime_name, $runtime_handler := .Runtimes  }}
  [crio.runtime.runtimes.{{ $runtime_name }}]
  runtime_path = "{{ $runtime_handler.RuntimePath }}"
  runtime_type = "{{ $runtime_handler.RuntimeType }}"
  {{ end  }}


# The crio.image table contains settings pertaining to the management of OCI images.
#
# CRI-O reads its configured registries defaults from the system wide
# containers-registries.conf(5) located in /etc/containers/registries.conf. If
# you want to modify just CRI-O, you can change the registries configuration in
# this file. Otherwise, leave insecure_registries and registries commented out to
# use the system's defaults from /etc/containers/registries.conf.
[crio.image]

# Default transport for pulling images from a remote container storage.
default_transport = "{{ .DefaultTransport }}"

# The path to a file containing credentials necessary for pulling images from
# secure registries. The file is similar to that of /var/lib/kubelet/config.json
global_auth_file = "{{ .GlobalAuthFile }}"

# The image used to instantiate infra containers.
pause_image = "{{ .PauseImage }}"

# The path to a file containing credentials specific for pulling the pause_image from
# above. The file is similar to that of /var/lib/kubelet/config.json
pause_image_auth_file = "{{ .PauseImageAuthFile }}"

# The command to run to have a container stay in the paused state.
pause_command = "{{ .PauseCommand }}"

# Path to the file which decides what sort of policy we use when deciding
# whether or not to trust an image that we've pulled. It is not recommended that
# this option be used, as the default behavior of using the system-wide default
# policy (i.e., /etc/containers/policy.json) is most often preferred. Please
# refer to containers-policy.json(5) for more details.
signature_policy = "{{ .SignaturePolicyPath }}"

# Controls how image volumes are handled. The valid values are mkdir, bind and
# ignore; the latter will ignore volumes entirely.
image_volumes = "{{ .ImageVolumes }}"

# List of registries to be used when pulling an unqualified image (e.g.,
# "alpine:latest"). By default, registries is set to "docker.io" for
# compatibility reasons. Depending on your workload and usecase you may add more
# registries (e.g., "quay.io", "registry.fedoraproject.org",
# "registry.opensuse.org", etc.).
#registries = [
# {{ range $opt := .Registries }}{{ printf "\t%q,\n#" $opt }}{{ end }}]


# The crio.network table containers settings pertaining to the management of
# CNI plugins.
[crio.network]

# Path to the directory where CNI configuration files are located.
network_dir = "{{ .NetworkDir }}"

# Paths to directories where CNI plugin binaries are located.
plugin_dirs = [
{{ range $opt := .PluginDirs }}{{ printf "\t%q,\n" $opt }}{{ end }}]
`))

// TODO: Currently ImageDir isn't really used, so we haven't added it to this
//       template. Add it once the storage code has been merged.

// NOTE: please propagate any changes to the template to docs/crio.conf.5.md

var configCommand = cli.Command{
	Name:  "config",
	Usage: "generate crio configuration files",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "default",
			Usage: "output the default configuration",
		},
	},
	Action: func(c *cli.Context) error {
		config, err := GetConfigFromContext(c)
		if err != nil {
			return err
		}

		if c.Bool("default") {
			config, err = server.DefaultConfig()
			if err != nil {
				return err
			}
		}

		// Validate the configuration during generation
		if err = config.Validate(false); err != nil {
			return err
		}

		// Output the commented config.
		return commentedConfigTemplate.ExecuteTemplate(os.Stdout, "config", config)
	},
}
