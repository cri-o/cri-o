% crio.conf(5) Kubernetes Container Runtime Daemon for Open Container Initiative Containers
% Aleksa Sarai
% OCTOBER 2016

# NAME
crio.conf - configuration file of the CRI-O OCI Kubernetes Container Runtime daemon

# DESCRIPTION
The CRI-O configuration file specifies all of the available configuration options and command-line flags for the [crio(8) OCI Kubernetes Container Runtime daemon][crio], but in a TOML format that can be more easily modified and versioned.

The default crio.conf is located at /etc/crio/crio.conf.

# FORMAT
The [TOML format][toml] is used as the encoding of the configuration file. Every option and subtable listed here is nested under a global "crio" table. No bare options are used. The format of TOML can be simplified to:

    [table]
    option = value

    [table.subtable1]
    option = value

    [table.subtable2]
    option = value

## CRIO TABLE
CRI-O reads its storage defaults from the containers-storage.conf(5) file located at /etc/containers/storage.conf. Modify this storage configuration if you want to change the system's defaults. If you want to modify storage just for CRI-O, you can change the storage configuration options here.

**root**="/var/lib/containers/storage"
  Path to the "root directory". CRI-O stores all of its data, including containers images, in this directory.

**runroot**="/var/run/containers/storage"
  Path to the "run directory". CRI-O stores all of its state in this directory.

**storage_driver**="overlay"
  Storage driver used to manage the storage of images and containers. Please refer to containers-storage.conf(5) to see all available storage drivers.

**storage_option**=[]
  List to pass options to the storage driver. Please refer to containers-storage.conf(5) to see all available storage options.

**file_locking**=true
  If set to false, in-memory locking will be used instead of file-based locking.

**file_locking_path**="/runc/crio.lock"
  Path to the lock file.


## CRIO.API TABLE
The `crio.api` table contains settings for the kubelet/gRPC interface.

**host_ip**=""
  Host IP considered as the primary IP to use by CRI-O for things such as host network IP.

**listen**="/var/run/crio/crio.sock"
  Path to AF_LOCAL socket on which CRI-O will listen.

**stream_address**="127.0.0.1"
  IP address on which the stream server will listen.

**stream_port**="0"
  The port on which the stream server will listen.

**stream_enable_tls**=false
  Enable encrypted TLS transport of the stream server.

**stream_tls_cert**=""
  Path to the x509 certificate file used to serve the encrypted stream. This file can change, and CRI-O will automatically pick up the changes within 5 minutes.

**stream_tls_key**=""
  Path to the key file used to serve the encrypted stream. This file can change, and CRI-O will automatically pick up the changes within 5 minutes.

**stream_tls_ca**=""
  Path to the x509 CA(s) file used to verify and authenticate client communication with the encrypted stream. This file can change, and CRI-O will automatically pick up the changes within 5 minutes.


## CRIO.RUNTIME TABLE
The `crio.runtime` table contains settings pertaining to the OCI runtime used and options for how to set up and manage the OCI runtime.

**no_pivot**=*false*
  If true, the runtime will not use `pivot_root`, but instead use `MS_MOVE`.

**conmon**="/usr/local/libexec/crio/conmon"
  Path to the conmon binary, used for monitoring the OCI runtime.

**conmon_env**=["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"]
  Environment variable list for the conmon process, used for passing necessary environment variables to conmon or the runtime.

**selinux**=false
  If true, SELinux will be used for pod separation on the host.

**seccomp_profile**="/etc/crio/seccomp.json"
  Path to the seccomp.json profile which is used as the default seccomp profile for the runtime.

**apparmor_profile**=""
  Used to change the name of the default AppArmor profile of CRI-O. The default profile name is "crio-default-" followed by the version string of CRI-O.

**cgroup_manager**="cgroupfs"
  Cgroup management implementation used for the runtime.

**default_capabilities**=[]
  List of default capabilities for containers. If it is empty or commented out, only the capabilities defined in the container json file by the user/kube will be added.

  The default list is:
```
  default_capabilities = [
          "CHOWN",
          "DAC_OVERRIDE",
          "FSETID",
          "FOWNER",
          "NET_RAW",
          "SETGID",
          "SETUID",
          "SETPCAP",
          "NET_BIND_SERVICE",
          "SYS_CHROOT",
          "KILL",
  ]
```

**default_sysctls**=[]
 List of default sysctls. If it is empty or commented out, only the sysctls defined in the container json file by the user/kube will be added.

**additional_devices**=[]
  List of additional devices. If it is empty or commented out, only the devices defined in the container json file by the user/kube will be added.

**hooks_dir**=["*path*", ...]
  Each `*.json` file in the path configures a hook for CRI-O containers.  For more details on the syntax of the JSON files and the semantics of hook injection, see `oci-hooks(5)`.  CRI-O currently support both the 1.0.0 and 0.1.0 hook schemas, although the 0.1.0 schema is deprecated.

  Paths listed later in the array higher precedence (`oci-hooks(5)` discusses directory precedence).

  For the annotation conditions, CRI-O uses the Kubernetes annotations, which are a subset of the annotations passed to the OCI runtime.  For example, `io.kubernetes.cri-o.Volumes` is part of the OCI runtime configuration annotations, but it is not part of the Kubernetes annotations being matched for hooks.

  For the bind-mount conditions, only mounts explicitly requested by Kubernetes configuration are considered.  Bind mounts that CRI-O inserts by default (e.g. `/dev/shm`) are not considered.

  If `hooks_dir` is unset, CRI-O will currently default to `/usr/share/containers/oci/hooks.d` and `/etc/containers/oci/hooks.d` in order of increasing precedence.  Using these defaults is deprecated, and callers should migrate to explicitly setting `hooks_dir`.

**default_mounts**=[]
  List of default mounts for each container. **Deprecated:** this option will be removed in future versions in favor of `default_mounts_file`.

**default_mounts_file**="/etc/containers/mounts.conf"
  Path to the file specifying the defaults mounts for each container. The format of the config is /SRC:/DST, one mount per line. Notice that CRI-O reads its default mounts from the following two files:

    1) `/etc/containers/mounts.conf` (i.e., default_mounts_file): This is the override file, where users can either add in their own default mounts, or override the default mounts shipped with the package.

    2) `/usr/share/containers/mounts.conf`: This is the default file read for mounts. If you want CRI-O to read from a different, specific mounts file, you can change the default_mounts_file. Note, if this is done, CRI-O will only add mounts it finds in this file.

**pids_limit**=1024
  Maximum number of processes allowed in a container.

**log_size_max**=-1
  Maximum size allowed for the container log file. Negative numbers indicate that no size limit is imposed. If it is positive, it must be >= 8192 to match/exceed conmon's read buffer. The file is truncated and re-opened so the limit is never exceeded.

**container_exits_dir**="/var/run/crio/exits"
  Path to directory in which container exit files are written to by conmon.

**container_attach_socket_dir**="/var/run/crio"
  Path to directory for container attach sockets.

**read_only**=false
  If set to true, all containers will run in read-only mode.

**log_level**="error"
  Changes the verbosity of the logs based on the level it is set to. Options are fatal, panic, error, warn, info, and debug.

**uid_mappings**=""
  The UID mappings for the user namespace of each container. A range is specified in the form containerUID:HostUID:Size. Multiple ranges must be separated by comma.

**gid_mappings**=""
  The GID mappings for the user namespace of each container. A range is specified in the form containerGID:HostGID:Size. Multiple ranges must be separated by comma.

**ctr_stop_timeout**=10
  The minimal amount of time in seconds to wait before issuing a timeout regarding the proper termination of the container.

### CRIO.RUNTIME.RUNTIMES TABLE
The "crio.runtime.runtimes" table defines a list of OCI compatible runtimes.  The runtime to use is picked based on the runtime_handler provided by the CRI.  If no runtime_handler is provided, the runtime will be picked based on the level of trust of the workload.

**runtime_path**=""
  Path to the OCI compatible runtime used for this runtime handler.

## CRIO.IMAGE TABLE
The `crio.image` table contains settings pertaining to the management of OCI images.

CRI-O reads its configured registries defaults from the system wide containers-registries.conf(5) located in /etc/containers/registries.conf. If you want to modify just CRI-O, you can change the registries configuration in this file. Otherwise, leave `insecure_registries` and `registries` commented out to use the system's defaults from /etc/containers/registries.conf.

**default_transport**="docker://"
  Default transport for pulling images from a remote container storage.

**global_auth_file**=""
  The path to a file like /var/lib/kubelet/config.json holding credentials necessary for pulling images from secure registries.

**pause_image**="k8s.gcr.io/pause:3.1"
  The image used to instantiate infra containers.

**pause_image_auth_file**=""
 The path to a file like /var/lib/kubelet/config.json holding credentials specific to pulling the pause_image from above.

**pause_command**="/pause"
  The command to run to have a container stay in the paused state.

**signature_policy**="/etc/containers/policy.json"
  Path to the file which decides what sort of policy we use when deciding whether or not to trust an image that we've pulled. It is not recommended that this option be used, as the default behavior of using the system-wide default policy (i.e., /etc/containers/policy.json) is most often preferred. Please refer to containers-policy.json(5) for more details.

**image_volumes**="mkdir"
  Controls how image volumes are handled. The valid values are mkdir, bind and ignore; the latter will ignore volumes entirely.

**insecure_registries**=[]
  List of registries to skip TLS verification for pulling images.

**registries**=["docker.io"]
  List of registries to be used when pulling an unqualified image (e.g., "alpine:latest"). By default, registries is set to "docker.io" for compatibility reasons. Depending on your workload and usecase you may add more registries (e.g., "quay.io", "registry.fedoraproject.org", "registry.opensuse.org", etc.).


## CRIO.NETWORK TABLE
The `crio.network` table containers settings pertaining to the management of CNI plugins.

**network_dir**="/etc/cni/net.d/"
  Path to the directory where CNI configuration files are located.

**plugin_dirs**=["/opt/cni/bin/",]
  List of paths to directories where CNI plugin binaries are located.

# SEE ALSO
containers-storage.conf(5), containers-policy.json(5), containers-registries.conf(5), crio(8)

# HISTORY
Aug 2018, Update to the latest state by Valentin Rothberg <vrothberg@suse.com>

Oct 2016, Originally compiled by Aleksa Sarai <asarai@suse.de>

[toml]: https://github.com/toml-lang/toml
[crio]: ./crio.8.md
