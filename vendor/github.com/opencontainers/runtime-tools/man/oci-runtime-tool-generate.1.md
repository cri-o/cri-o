% OCI(1) OCI-RUNTIME-TOOL User Manuals
% OCI Community
% APRIL 2016
# NAME
oci-runtime-tool-generate - Generate a config.json for an OCI container

# SYNOPSIS
**oci-runtime-tool generate** *[OPTIONS]*

# DESCRIPTION

`oci-runtime-tool generate` generates configuration JSON for an OCI bundle.
By default, it writes the JSON to stdout, but you can use **--output**
to direct it to a file.  OCI-compatible runtimes like runC expect to
read the configuration from `config.json`.

# OPTIONS
**--apparmor**=PROFILE
  Specifies the apparmor profile for the container

**--arch**=ARCH
  Architecture used within the container.
  "amd64"

**--args**=OPTION
  Arguments to run within the container.  Can be specified multiple times.
  If you were going to run a command with multiple options, you would need
  to specify the command and each argument in order.

  --args "/usr/bin/httpd" --args "-D" --args "FOREGROUND"

**--bind**=*[[HOST-DIR:CONTAINER-DIR][:OPTIONS...]]*
  Bind mount directories src:dest:(rw,ro) If you specify, ` --bind
  /HOST-DIR:/CONTAINER-DIR`, runc bind mounts `/HOST-DIR` in the host
  to `/CONTAINER-DIR` in the OCI container. The `OPTIONS` are a colon
  delimited list and can be any mount option support by the runtime such
  as [rw|ro|rbind|bind|...]. The `HOST_DIR` and `CONTAINER-DIR` must be
  absolute paths such as `/src/docs`.  You can set the `ro` or `rw`
  options to a bind-mount to mount it read-only or read-write mode,
  respectively. By default, bind-mounts are mounted read-write.

**--cap-add**=[]
  Add Linux capabilities

**--cap-drop**=[]
  Drop Linux capabilities

**--cgroup**=*PATH*
  Use a Cgroup namespace where *PATH* is an existing Cgroup namespace file
  to join. The special *PATH*  empty-string  creates a new namespace.
  The special *PATH* `host` removes any existing Cgroup namespace from
  the configuration.

**--cgroups-path**=""
  Specifies the path to the cgroups relative to the cgroups mount point.

**--cwd**=PATH
  Current working directory for the process. The deafult is */*.

**--disable-oom-kill**=true|false
  Whether to disable OOM Killer for the container or not.

**--env**=[]
  Set environment variables e.g. key=value.
  This option allows you to specify arbitrary environment variables
  that are available for the process that will be launched inside of
  the container.
  
**--env-file**=[]
  Set environment variables from a file.
  This option sets environment variables in the container from the
  contents of a file formatted with key=value pairs, one per line.
  When specified multiple times, files are loaded in order with duplicate
  keys overwriting previous ones.

**--gid**=GID
  Gid for the process inside of container

**--gidmappings**=GIDMAPPINGS
  Add GIDMappings e.g HostID:ContainerID:Size.  Implies **-user=**.

**--groups**=GROUP
  Supplementary groups for the processes inside of container

**--help**
  Print usage statement

**--hostname**=""
  Set the container host name that is available inside the container.

**--ipc**=*PATH*
  Use an IPC namespace where *PATH* is an existing IPC namespace file
  to join. The special *PATH*  empty-string  creates a new namespace.
  The special *PATH* `host` removes any existing IPC namespace from the
  configuration.

**--label**=[]
  Add annotations to the configuration e.g. key=value.
  Currently, key containing equals sign is not supported.

**--linux-cpu-shares**=CPUSHARES
  Specifies a relative share of CPU time available to the tasks in a cgroup.

**--linux-cpu-period**=CPUPERIOD
  Specifies a period of time in microseconds for how regularly a cgroup's access to CPU resources should be reallocated (CFS scheduler only).

**--linux-cpu-quota**=CPUQUOTA
  Specifies the total amount of time in microseconds for which all tasks in a cgroup can run during one period.

**--linux-cpus**=CPUS
  Sets the CPUs to use within the cpuset (default is to use any CPU available).

**--linux-mem-kernel-limit**=MEMKERNELLIMIT
  Sets the hard limit of kernel memory in bytes.

**--linux-mem-kernel-tcp**=MEMKERNELTCP
  Sets the hard limit of kernel TCP buffer memory in bytes.

**--linux-mem-limit**=MEMLIMIT
  Sets the limit of memory usage in bytes.

**--linux-mem-reservation**=MEMRESERVATION
  Sets the soft limit of memory usage in bytes.

**--linux-mem-swap**=MEMSWAP
  Sets the total memory limit (memory + swap) in bytes.

**--linux-mem-swappiness**=MEMSWAPPINESS
  Sets the swappiness of how the kernel will swap memory pages (Range from 0 to 100).

**--linux-mems**=MEMS
  Sets the list of memory nodes in the cpuset (default is to use any available memory node).

**--linux-network-classid**=CLASSID
  Specifies network class identifier which will be tagged by container's network packets.

**--linux-network-priorities**=[]
  Specifies network priorities of network traffic, format is NAME:PRIORITY.
  e.g. --linux-network-priorities=eth0:123
  This option can be specified multiple times. If a interface name was specified more than once, the last PRIORITY makes sense.
  The special *PRIORITY*  -1  removes existing setting for interface NAME.

**--linux-pids-limit**=PIDSLIMIT
  Set maximum number of PIDs.

**--linux-realtime-period**=REALTIMEPERIOD
  Sets the CPU period to be used for realtime scheduling (in usecs). Same as **--linux-cpu-period** but applies to realtime scheduler only.

**--linux-realtime-runtime**=REALTIMERUNTIME
  Specifies a period of time in microseconds for the longest continuous period in which the tasks in a cgroup have access to CPU resources.

**--masked-paths**=[]
  Specifies paths can not be read inside container. e.g. --masked-paths=/proc/kcore
  This option can be specified multiple times.

**--mount**=*PATH*
  Use a mount namespace where *PATH* is an existing mount namespace file
  to join. The special *PATH*  empty-string  creates a new namespace.
  The special *PATH* `host` removes any existing mount namespace from the
  configuration.

**--mount-cgroups**=[rw|ro|no]
  Mount cgroups. The default is *no*.

**--mount-label**=MOUNTLABEL
  Mount Label
  Depending on your SELinux policy, you would specify a label that looks like
  this:
  "system_u:object_r:svirt_sandbox_file_t:s0:c1,c2"

    Note you would want your ROOTFS directory to be labeled with a context that
    this process type can use.

      "system_u:object_r:usr_t:s0" might be a good label for a readonly container,
      "system_u:system_r:svirt_sandbox_file_t:s0:c1,c2" for a read/write container.

**--network**=*PATH*
  Use a network namespace where *PATH* is an existing network namespace file
  to join. The special *PATH*  empty-string  creates a new namespace.
  The special *PATH* `host` removes any existing network namespace from the
  configuration.

**--no-new-privileges**=true|false
  Set no new privileges bit for the container process.  Setting this flag
  will block the container processes from gaining any additional privileges
  using tools like setuid apps.  It is a good idea to run unprivileged
  containers with this flag.

**--oom-score-adj**=adj
  Specifies oom_score_adj for the container.

**--os**=OS
  Operating system used within the container.

**--output**=PATH
  Instead of writing the configuration JSON to stdout, write it to a
  file at *PATH* (overwriting the existing content if a file already
  exists at *PATH*).

**--pid**=*PATH*
  Use a PID namespace where *PATH* is an existing PID namespace file
  to join. The special *PATH*  empty-string  creates a new namespace.
  The special *PATH* `host` removes any existing PID namespace from
  the configuration.

**--poststart**=CMD[:ARGS...]
  Set command to run in poststart hooks. Can be specified multiple times.
  The multiple commands will be run in order before the container process
  gets launched but after the container environment and main process has been
  created.

**--poststop**=CMD[:ARGS...]
  Set command to run in poststop hooks. Can be specified multiple times.
  The multiple commands will be run in order after the container process
  is stopped.

**--prestart**=CMD[:ARGS...]
  Set command to run in prestart hooks. Can be specified multiple times.
  The multiple commands will be run in order after the container process
  has been created but before it executes the user-configured code.

**--privileged**=true|false
  Give extended privileges to this container. The default is *false*.

  By default, OCI containers are
“unprivileged” (=false) and cannot do some of the things a normal root process can do.

  When the operator executes **oci-runtime-tool generate --privileged**, OCI will enable access to all devices on the host as well as disable some of the confinement mechanisms like AppArmor, SELinux, and seccomp from blocking access to privileged processes.  This gives the container processes nearly all the same access to the host as processes generating outside of a container on the host.

**--readonly-paths**=[]
  Specifies paths readonly inside container. e.g. --readonly-paths=/proc/sys
  This option can be specified multiple times.

**--rootfs-path**=ROOTFSPATH
  Path to the rootfs, which can be an absolute path or relative to bundle path.
  e.g the absolute path of rootfs is /to/bundle/rootfs, bundle path is /to/bundle,
  then the value set as ROOTFSPATH should be `/to/bundle/rootfs` or `rootfs`. The default is *rootfs*.

**--rootfs-propagation**=PROPOGATIONMODE
  Mount propagation for root filesystem.
  Values are "shared, rshared, private, rprivate, slave, rslave"

**--rootfs-readonly**=true|false
  Mount the container's root filesystem as read only.

  By default a container will have its root filesystem writable allowing processes to write files anywhere.  By specifying the `--rootfs-readonly` flag the container will have its root filesystem mounted as read only prohibiting any writes.

**--rlimits-add**=[]
  Specifies resource limits, format is RLIMIT:HARD:SOFT. e.g. --rlimits-add=RLIMIT_NOFILE:1024:1024
  This option can be specified multiple times. When same RLIMIT specified over once, the last one make sense.

**--rlimits-remove**=[]
  Remove the specified resource limits for process inside the container.
  This option can be specified multiple times.

**--rlimits-remove-all**=true|false
  Remove all resource limits for process inside the container. The default is *false*.

**--seccomp-allow**=SYSCALL
  Specifies syscalls to be added to the ALLOW list.
  See --seccomp-syscalls for setting limits on arguments.

**--seccomp-arch**=ARCH
  Specifies Additional architectures permitted to be used for system calls.
  By default if you turn on seccomp, only the host architecture will be allowed.

**--seccomp-default**=ACTION
  Specifies the the default action of Seccomp syscall restrictions and removes existing restrictions with the specified action
  Values: KILL,ERRNO,TRACE,ALLOW

**--seccomp-default-force**=ACTION
  Specifies the the default action of Seccomp syscall restrictions
  Values: KILL,ERRNO,TRACE,ALLOW

**--seccomp-errno**=SYSCALL
  Specifies syscalls to create seccomp rule to respond with ERRNO.

**--seccomp-kill**=SYSCALL
  Specifies syscalls to create seccomp rule to respond with KILL.

**--seccomp-only**
  Option to only export the seccomp section of output

**--seccomp-remove**
  Specifies syscall restrictions to remove from the configuration.

**--seccomp-remove-all**
  Option to remove all syscall restrictions.

**--seccomp-trace**=SYSCALL
  Specifies syscalls to create seccomp rule to respond with TRACE.

**--seccomp-trap**=SYSCALL
  Specifies syscalls to create seccomp rule to respond with TRAP.

**--selinux-label**=PROCESSLABEL
  SELinux Label
  Depending on your SELinux policy, you would specify a label that looks like
  this:
  "system_u:system_r:svirt_lxc_net_t:s0:c1,c2"

    Note you would want your ROOTFS directory to be labeled with a context that
    this process type can use.

      "system_u:object_r:usr_t:s0" might be a good label for a readonly container,
      "system_u:object_r:svirt_sandbox_file_t:s0:c1,c2" for a read/write container.

**--sysctl**=SYSCTLSETTING
  Add sysctl settings e.g net.ipv4.forward=1, only allowed if the syctl is
  namespaced.

**--template**=PATH
  Override the default template with your own.
  Additional options will only adjust the relevant portions of your template.

**--tmpfs**=[] Create a tmpfs mount
  Mount a temporary filesystem (`tmpfs`) mount into a container, for example:

    $ oci-runtime-tool generate -d --tmpfs /tmp:rw,size=787448k,mode=1777 my_image

    This command mounts a `tmpfs` at `/tmp` within the container.  The supported mount options are the same as the Linux default `mount` flags. If you do not specify any options, the systems uses the following options:
    `rw,noexec,nosuid,nodev,size=65536k`.

**--tty**=true|false
  Allocate a new tty for the container process. The default is *false*.

**--uid**=UID
  Sets the UID used within the container.

**--uidmappings**
  Add UIDMappings e.g HostUID:ContainerID:Size.  Implies **--user=**.

**--user**=*PATH*
  Use a user namespace where *PATH* is an existing user namespace file
  to join. The special *PATH*  empty-string  creates a new namespace.
  The special *PATH* `host` removes any existing user namespace from
  the configuration.

**--uts**=*PATH*
  Use a UTS namespace where *PATH* is an existing UTS namespace file
  to join. The special *PATH*  empty-string  creates a new namespace.
  The special *PATH* `host` removes any existing UTS namespace from
  the configuration.

# EXAMPLES

## Generating container in read-only mode

During container image development, containers often need to write to the image
content.  Installing packages into /usr, for example.  In production,
applications seldom need to write to the image.  Container applications write
to volumes if they need to write to file systems at all.  Applications can be
made more secure by generating them in read-only mode using the --read-only switch.
This protects the containers image from modification. Read only containers may
still need to write temporary data.  The best way to handle this is to mount
tmpfs directories on /generate and /tmp.

    # oci-runtime-tool generate --read-only --tmpfs /generate --tmpfs /tmp --tmpfs /run  --rootfs /var/lib/containers/fedora /bin/bash

## Exposing log messages from the container to the host's log

If you want messages that are logged in your container to show up in the host's
syslog/journal then you should bind mount the /dev/log directory as follows.

    # oci-runtime-tool generate --bind /dev/log:/dev/log  --rootfs /var/lib/containers/fedora /bin/bash

From inside the container you can test this by sending a message to the log.

    (bash)# logger "Hello from my container"

Then exit and check the journal.

    # exit

    # journalctl -b | grep Hello

This should list the message sent to logger.

## Bind Mounting External Volumes

To mount a host directory as a container volume, specify the absolute path to
the directory and the absolute path for the container directory separated by a
colon:

    # oci-runtime-tool generate --bind /var/db:/data1  --rootfs /var/lib/containers/fedora --args bash

## Using SELinux

You can use SELinux to add security to the container.  You must specify the process label to run the init process inside of the container using the --selinux-label.

    # oci-runtime-tool generate --bind /var/db:/data1  --selinux-label system_u:system_r:svirt_lxc_net_t:s0:c1,c2 --mount-label system_u:object_r:svirt_sandbox_file_t:s0:c1,c2 --rootfs /var/lib/containers/fedora --args bash

Not in the above example we used a type of svirt_lxc_net_t and an MCS Label of s0:c1,c2.  If you want to guarantee separation between containers, you need to make sure that each container gets launched with a different MCS Label pair.

Also the underlying rootfs must be labeled with a matching label.  For the example above, you would execute a command like:

    # chcon -R system_u:object_r:svirt_sandbox_file_t:s0:c1,c2  /var/lib/containers/fedora

This will set up the labeling of the rootfs so that the process launched would be able to write to the container.  If you wanted to only allow it to read/execute the content in rootfs, you could execute:

    # chcon -R system_u:object_r:usr_t:s0  /var/lib/containers/fedora

When using SELinux, be aware that the host has no knowledge of container SELinux
policy. Therefore, in the above example, if SELinux policy is enforced, the
`/var/db` directory is not writable to the container. A "Permission Denied"
message will occur and an avc: message in the host's syslog.

To work around this, the following command needs to be generate in order for the proper SELinux policy type label to be attached to the host directory:

    # chcon -Rt svirt_sandbox_file_t -l s0:c1,c2 /var/db

Now, writing to the /data1 volume in the container will be allowed and the
changes will also be reflected on the host in /var/db.

# SEE ALSO
**runc**(1), **oci-runtime-tool**(1)

# HISTORY
April 2016, Originally compiled by Dan Walsh (dwalsh at redhat dot com)
