package main

import (
	"os"
	"text/template"

	"github.com/kubernetes-sigs/cri-o/server"
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


# The crio.runtime table contains settings pertaining to the OCI runtime used
# and options for how to set up and manage the OCI runtime.
[crio.runtime]

# Path to the OCI compatible runtime used for trusted container workloads. This
# is a mandatory setting as this runtime will be the default and will also be
# used for untrusted container workloads if runtime_untrusted_workload is not
# set.
#
# DEPRECATED: use Runtimes instead.
#
# runtime = "{{ .Runtime }}"

# Path to OCI compatible runtime used for untrusted container workloads. This
# is an optional setting, except if default_container_trust is set to
# "untrusted".
# DEPRECATED: use "crio.runtime.runtimes" instead. If provided, this
#     runtime is mapped to the runtime handler named 'untrusted'. It is
#     a configuration error to provide both the (now deprecated)
#     runtime_untrusted_workload and a handler in the Runtimes handler
#     map (below) for 'untrusted' workloads at the same time. Please
#     provide one or the other.
#     The support of this option will continue through versions 1.12 and 1.13.
#     By version 1.14, this option will no longer exist.
#runtime_untrusted_workload = "{{ .RuntimeUntrustedWorkload }}"

# Default level of trust CRI-O puts in container workloads. It can either be
# "trusted" or "untrusted", and the default is "trusted". Containers can be run
# through different container runtimes, depending on the trust hints we receive
# from kubelet:
#
#   - If kubelet tags a container workload as untrusted, CRI-O will try first
#     to run it through the untrusted container workload runtime. If it is not
#     set, CRI-O will use the trusted runtime.
#
#   - If kubelet does not provide any information about the container workload
#     trust level, the selected runtime will depend on the default_container_trust
#     setting. If it is set to untrusted, then all containers except for the host
#     privileged ones, will be run by the runtime_untrusted_workload runtime. Host
#     privileged containers are by definition trusted and will always use the
#     trusted container runtime. If default_container_trust is set to "trusted",
#     CRI-O will use the trusted container runtime for all containers.
#
# DEPRECATED: The runtime handler should provide a key to the map of runtimes,
#     avoiding the need to rely on the level of trust of the workload to choose
#     an appropriate runtime.
#     The support of this option will continue through versions 1.12 and 1.13.
#     By version 1.14, this option will no longer exist.
#default_workload_trust = "{{ .DefaultWorkloadTrust }}"

  # The "crio.runtime.runtimes" table defines a list of OCI compatible runtimes.
  # The runtime to use is picked based on the runtime_handler provided by the CRI.
  # If no runtime_handler is provided, the runtime will be picked based on the level
  # of trust of the workload.
  {{ range $runtime_name, $runtime_path := .Runtimes  }}
  [crio.runtime.runtimes.{{ $runtime_name }}]
  runtime_path = "{{ $runtime_path.RuntimePath }}"
  {{ end  }}

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

# Path to the OCI hooks directory for automatically executed hooks.
hooks_dir_path = "{{ .HooksDirPath }}"

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


# The crio.image table contains settings pertaining to the management of OCI images.
#
# CRI-O reads its configured registries defaults from the system wide
# containers-registries.conf(5) located in /etc/containers/registries.conf. If
# you want to modify just CRI-O, you can change the registies configuration in
# this file. Otherwise, leave insecure_registries and registries commented out to
# use the system's defaults from /etc/containers/registries.conf.
[crio.image]

# Default transport for pulling images from a remote container storage.
default_transport = "{{ .DefaultTransport }}"

# The image used to instantiate infra containers.
pause_image = "{{ .PauseImage }}"

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

# Path to directory where CNI plugin binaries are located.
plugin_dir = "{{ .PluginDir }}"
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
		// At this point, app.Before has already parsed the user's chosen
		// config file. So no need to handle that here.
		config := c.App.Metadata["config"].(*server.Config)
		if c.Bool("default") {
			config = server.DefaultConfig()
		}

		// Output the commented config.
		return commentedConfigTemplate.ExecuteTemplate(os.Stdout, "config", config)
	},
}
