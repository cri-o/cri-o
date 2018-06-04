% crio(8) Open Container Initiative Daemon
% Dan Walsh
% SEPTEMBER 2016
# NAME
crio - OCI Kubernetes Container Runtime daemon

# SYNOPSIS
crio
```
[--apparmor-profile=[value]]
[--bind-mount-prefix=[value]]
[--cgroup-manager=[value]]
[--cni-config-dir=[value]]
[--cni-plugin-dir=[value]]
[--config=[value]]
[--conmon=[value]]
[--cpu-profile=[value]]
[--default-transport=[value]]
[--gid-mappings=[value]]
[--help|-h]
[--insecure-registry=[value]]
[--listen=[value]]
[--log=[value]]
[--log-format value]
[--log-level value]
[--pause-command=[value]]
[--pause-image=[value]]
[--read-only]
[--registry=[value]]
[--root=[value]]
[--runroot=[value]]
[--runtime=[value]]
[--seccomp-profile=[value]]
[--selinux]
[--signature-policy=[value]]
[--storage-driver=[value]]
[--storage-opt=[value]]
[--uid-mappings=[value]]
[--version|-v]
```
# DESCRIPTION
OCI-based implementation of Kubernetes Container Runtime Interface Daemon

crio is meant to provide an integration path between OCI conformant runtimes and the kubelet. Specifically, it implements the Kubelet Container Runtime Interface (CRI) using OCI conformant runtimes. The scope of crio is tied to the scope of the CRI.

1. Support multiple image formats including the existing Docker image format.
2. Support for multiple means to download images including trust & image verification.
3. Container image management (managing image layers, overlay filesystems, etc).
4. Container process lifecycle management.
5. Monitoring and logging required to satisfy the CRI.
6. Resource isolation as required by the CRI.

**Usage**:
```
crio [GLOBAL OPTIONS]
crio [GLOBAL OPTIONS] config [OPTIONS]
```
# GLOBAL OPTIONS
**--apparmor_profile**="": Name of the apparmor profile to be used as the runtime's default (default: "crio-default")

**--bind-mount-prefix**="": A prefix to use for the source of the bind mounts.  This option would be useful if you were running CRI-O in a container.  And had `/` mounted on `/host` in your container.  Then if you ran CRI-O with the `--bind-mount-prefix=/host` option, CRI-O would add /host to any bind mounts it is handed over CRI.  If Kubernetes asked to have `/var/lib/foobar` bind mounted into the container, then CRI-I would bind mount `/host/var/lib/foobar`.  Since CRI-O itself is running in a container with `/` or the host mounted on `/host`, the container would end up with `/var/lib/foobar` from the host mounted in the container rather then `/var/lib/foobar` from the CRI-O container.

**--cgroup-manager**="": cgroup manager (cgroupfs or systemd)

**--cni-config-dir**="": CNI configuration files directory (default: "/etc/cni/net.d/")

**--cni-plugin-dir**="": CNI plugin binaries directory (default: "/opt/cni/bin/")

**--config**="": path to configuration file

**--conmon**="": path to the conmon executable (default: "/usr/local/libexec/crio/conmon")

**--cpu-profile**="": set the CPU profile file path

**--default-transport**: A prefix to prepend to image names that can't be pulled as-is.

**--gid-mappings**: Specify the GID mappings to use for user namespace.

**--help, -h**: Print usage statement

**--insecure-registry=**: Enable insecure registry communication, i.e., enable un-encrypted and/or untrusted communication.

1. List of insecure registries can contain an element with CIDR notation to specify a whole subnet.
2. Insecure registries accept HTTP or accept HTTPS with certificates from unknown CAs.
3. Enabling `--insecure-registry`  is useful when running a local registry. However, because its use creates security vulnerabilities, **it should ONLY be enabled for testing purposes**. For increased security, users should add their CA to their system's list of trusted CAs instead of using `--insecure-registry`.

**--image-volumes**="": Image volume handling ('mkdir', 'bind' or 'ignore') (default: "mkdir")

1. mkdir: A directory is created inside the container root filesystem for the volumes.
2. bind: A directory is created inside container state directory and bind mounted into the container for the volumes.
3. ignore: All volumes are just ignored and no action is taken.

**--listen**="": Path to CRI-O socket (default: "/var/run/crio/crio.sock")

**--log**="": Set the log file path where internal debug information is written

**--log-format**="": Set the format used by logs ('text' (default), or 'json') (default: "text")

**--log-level**="": log crio messages above specified level: debug, info (default), warn, error, fatal or panic

**--log-size-max**="": Maximum log size in bytes for a container (default: -1 (no limit)). If it is positive, it must be >= 8192 (to match/exceed conmon read buffer).

**--pause-command**="": Path to the pause executable in the pause image (default: "/pause")

**--pause-image**="": Image which contains the pause executable (default: "kubernetes/pause")

**--pids-limit**="": Maximum number of processes allowed in a container (default: 1024)

**--read-only**=**true**|**false**: Run all containers in read-only mode (default: false). Automatically mount tmpfs on `/run`, `/tmp` and `/var/tmp`.

**--root**="": The crio root dir (default: "/var/lib/containers/storage")

**--registry**="": Registry host which will be prepended to unqualified images, can be specified multiple times

**--runroot**="": The crio state dir (default: "/var/run/containers/storage")

**--runtime**="": OCI runtime path (default: "/usr/bin/runc")

**--selinux**=**true**|**false**: Enable selinux support (default: false)

**--seccomp-profile**="": Path to the seccomp json profile to be used as the runtime's default (default: "/usr/lib/crio/seccomp.json")

**--signature-policy**="": Path to the signature policy json file (default: "", to use the system-wide default)

**--storage-driver**: OCI storage driver (default: "devicemapper")

**--storage-opt**: OCI storage driver option (no default)

**--uid-mappings**: Specify the UID mappings to use for user namespace.

**--version, -v**: Print the version

# COMMANDS
CRI-O's default command is to start the daemon. However, it currently offers a
single additional subcommand.

## config

Outputs a commented version of the configuration file that would've been used
by CRI-O. This allows you to save you current configuration setup and then load
it later with **--config**. Global options will modify the output.

**--default**
  Output the default configuration (without taking into account any configuration options).

## FILES

**crio.conf** (`/etc/crio/crio.conf`)
  `cri-o` configuration file for all of the available command-line options for the crio(8) program, but in a TOML format that can be more easily modified and versioned.

**policy.json** (`/etc/containers/policy.json`)
  Signature verification policy files are used to specify policy, e.g. trusted keys, applicable when deciding whether to accept an image, or individual signatures of that image, as valid.

**registries.conf** (`/etc/containers/registries.conf`)
  Registry configuration file specifies registries which are consulted when completing image names that do not include a registry or domain portion.

**storage.conf** (`/etc/containers/storage.conf`)
  Storage configuration file specifies all of the available container storage options for tools using shared container storage.

# SEE ALSO
crio.conf(5),policy.json(5),registries.conf(5),storage.conf(5)

# HISTORY
Sept 2016, Originally compiled by Dan Walsh <dwalsh@redhat.com> and Aleksa Sarai <asarai@suse.de>
