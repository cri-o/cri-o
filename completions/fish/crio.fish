# crio fish shell completion

function __fish_crio_no_subcommand --description 'Test if there has been any subcommand yet'
    for i in (commandline -opc)
        if contains -- $i check complete completion help h config man markdown md status config c containers container cs s info i goroutines g heap hp version wipe help h
            return 1
        end
    end
    return 0
end

complete -c crio -n '__fish_crio_no_subcommand' -f -l absent-mount-sources-to-reject -r -d 'A list of paths that, when absent from the host, will cause a container creation to fail (as opposed to the current behavior of creating a directory).'
complete -c crio -n '__fish_crio_no_subcommand' -f -l add-inheritable-capabilities -d 'Add capabilities to the inheritable set, as well as the default group of permitted, bounding and effective.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l additional-devices -r -d 'Devices to add to the containers.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l allowed-devices -r -d 'Devices a user is allowed to specify with the "io.kubernetes.cri-o.Devices" allowed annotation.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l apparmor-profile -r -d 'Name of the apparmor profile to be used as the runtime\'s default. This only takes effect if the user does not specify a profile via the Kubernetes Pod\'s metadata annotation.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l auto-reload-registries -d 'If true, CRI-O will automatically reload the mirror registry when there is an update to the \'registries.conf.d\' directory. Default value is set to \'false\'.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l big-files-temporary-dir -r -d 'Path to the temporary directory to use for storing big files, used to store image blobs and data streams related to containers image management.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l bind-mount-prefix -r -d 'A prefix to use for the source of the bind mounts. This option would be useful if you were running CRI-O in a container. And had \'/\' mounted on \'/host\' in your container. Then if you ran CRI-O with the \'--bind-mount-prefix=/host\' option, CRI-O would add /host to any bind mounts it is handed over CRI. If Kubernetes asked to have \'/var/lib/foobar\' bind mounted into the container, then CRI-O would bind mount \'/host/var/lib/foobar\'. Since CRI-O itself is running in a container with \'/\' or the host mounted on \'/host\', the container would end up with \'/var/lib/foobar\' from the host mounted in the container rather then \'/var/lib/foobar\' from the CRI-O container.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l blockio-config-file -r -d 'Path to the blockio class configuration file for configuring the cgroup blockio controller.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l blockio-reload -d 'Reload blockio-config-file and rescan blockio devices in the system before applying blockio parameters.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l cdi-spec-dirs -r -d 'Directories to scan for CDI Spec files.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l cgroup-manager -r -d 'cgroup manager (cgroupfs or systemd).'
complete -c crio -n '__fish_crio_no_subcommand' -l clean-shutdown-file -r -d 'Location for CRI-O to lay down the clean shutdown file. It indicates whether we\'ve had time to sync changes to disk before shutting down. If not found, crio wipe will clear the storage directory.'
complete -c crio -n '__fish_crio_no_subcommand' -l cni-config-dir -r -d 'CNI configuration files directory.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l cni-default-network -r -d 'Name of the default CNI network to select. If not set or "", then CRI-O will pick-up the first one found in --cni-config-dir.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l cni-plugin-dir -r -d 'CNI plugin binaries directory.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l collection-period -r -d 'The number of seconds between collecting pod/container stats and pod sandbox metrics. If set to 0, the metrics/stats are collected on-demand instead.'
complete -c crio -n '__fish_crio_no_subcommand' -l config -s c -r -d 'Path to configuration file'
complete -c crio -n '__fish_crio_no_subcommand' -l config-dir -s d -r -d 'Path to the configuration drop-in directory.
    This directory will be recursively iterated and each file gets applied
    to the configuration in their processing order. This means that a
    configuration file named \'00-default\' has a lower priority than a file
    named \'01-my-overwrite\'.
    The global config file, provided via \'--config,-c\' or per default in
    /etc/crio/crio.conf, always has a lower priority than the files in the directory specified
    by \'--config-dir,-d\'.
    Besides that, provided command line parameters have a higher priority
    than any configuration file.'
complete -c crio -n '__fish_crio_no_subcommand' -l conmon -r -d 'Path to the conmon binary, used for monitoring the OCI runtime. Will be searched for using $PATH if empty. This option is deprecated, and will be removed in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l conmon-cgroup -r -d 'cgroup to be used for conmon process. This option is deprecated and will be removed in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l conmon-env -r -d 'Environment variable list for the conmon process, used for passing necessary environment variables to conmon or the runtime. This option is deprecated and will be removed in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -l container-attach-socket-dir -r -d 'Path to directory for container attach sockets.'
complete -c crio -n '__fish_crio_no_subcommand' -l container-exits-dir -r -d 'Path to directory in which container exit files are written to by conmon.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l ctr-stop-timeout -r -d 'The minimal amount of time in seconds to wait before issuing a timeout regarding the proper termination of the container. The lowest possible value is 30s, whereas lower values are not considered by CRI-O.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l decryption-keys-path -r -d 'Path to load keys for image decryption.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l default-capabilities -r -d 'Capabilities to add to the containers.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l default-env -r -d 'Additional environment variables to set for all containers.'
complete -c crio -n '__fish_crio_no_subcommand' -l default-mounts-file -r -d 'Path to default mounts file.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l default-runtime -r -d 'Default OCI runtime from the runtimes config.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l default-sysctls -r -d 'Sysctls to add to the containers.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l default-transport -r -d 'A prefix to prepend to image names that cannot be pulled as-is.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l default-ulimits -r -d 'Ulimits to apply to containers by default (name=soft:hard).'
complete -c crio -n '__fish_crio_no_subcommand' -f -l device-ownership-from-security-context -d 'Set devices\' uid/gid ownership from runAsUser/runAsGroup.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l disable-hostport-mapping -d 'If true, CRI-O would disable the hostport mapping.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l drop-infra-ctr -d 'Determines whether pods are created without an infra container, when the pod is not using a pod level PID namespace.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l enable-criu-support -d 'Enable CRIU integration, requires that the criu binary is available in $PATH.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l enable-metrics -d 'Enable metrics endpoint for the server.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l enable-nri -d 'Enable NRI (Node Resource Interface) support. (default: true)'
complete -c crio -n '__fish_crio_no_subcommand' -f -l enable-pod-events -d 'If true, CRI-O starts sending the container events to the kubelet'
complete -c crio -n '__fish_crio_no_subcommand' -f -l enable-profile-unix-socket -d 'Enable pprof profiler on crio unix domain socket.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l enable-tracing -d 'Enable OpenTelemetry trace data exporting.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l gid-mappings -r -d 'Specify the GID mappings to use for the user namespace. This option is deprecated, and will be replaced with Kubernetes user namespace (KEP-127) support in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -l global-auth-file -r -d 'Path to a file like /var/lib/kubelet/config.json holding credentials necessary for pulling images from secure registries.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l grpc-max-recv-msg-size -r -d 'Maximum grpc receive message size in bytes.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l grpc-max-send-msg-size -r -d 'Maximum grpc receive message size.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l hooks-dir -r -d 'Set the OCI hooks directory path (may be set multiple times)
    If one of the directories does not exist, then CRI-O will automatically
    skip them.
    Each \'\*.json\' file in the path configures a hook for CRI-O
    containers. For more details on the syntax of the JSON files and
    the semantics of hook injection, see \'oci-hooks(5)\'. CRI-O
    currently support both the 1.0.0 and 0.1.0 hook schemas, although
    the 0.1.0 schema is deprecated.
    This option may be set multiple times; paths from later options
    have higher precedence (\'oci-hooks(5)\' discusses directory
    precedence).
    For the annotation conditions, CRI-O uses the Kubernetes
    annotations, which are a subset of the annotations passed to the
    OCI runtime. For example, \'io.kubernetes.cri-o.Volumes\' is part of
    the OCI runtime configuration annotations, but it is not part of
    the Kubernetes annotations being matched for hooks.
    For the bind-mount conditions, only mounts explicitly requested by
    Kubernetes configuration are considered. Bind mounts that CRI-O
    inserts by default (e.g. \'/dev/shm\') are not considered.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l hostnetwork-disable-selinux -d 'Determines whether SELinux should be disabled within a pod when it is running in the host network namespace.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l image-volumes -r -d 'Image volume handling (\'mkdir\', \'bind\', or \'ignore\')
    1. mkdir: A directory is created inside the container root filesystem for
       the volumes.
    2. bind: A directory is created inside container state directory and bind
       mounted into the container for the volumes.
	3. ignore: All volumes are just ignored and no action is taken.'
complete -c crio -n '__fish_crio_no_subcommand' -l imagestore -r -d 'Store newly pulled images in the specified path, rather than the path provided by --root.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l included-pod-metrics -r -d 'A list of pod metrics to include. Specify the names of the metrics to include in this list.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l infra-ctr-cpuset -r -d 'CPU set to run infra containers, if not specified CRI-O will use all online CPUs to run infra containers.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l insecure-registry -r -d 'Enable insecure registry communication, i.e., enable un-encrypted and/or untrusted communication.
    1. List of insecure registries can contain an element with CIDR notation to
       specify a whole subnet.
    2. Insecure registries accept HTTP or accept HTTPS with certificates from
       unknown CAs.
    3. Enabling \'--insecure-registry\' is useful when running a local registry.
       However, because its use creates security vulnerabilities, **it should ONLY
       be enabled for testing purposes**. For increased security, users should add
       their CA to their system\'s list of trusted CAs instead of using
       \'--insecure-registry\'.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l internal-repair -d 'If true, CRI-O will check if the container and image storage was corrupted after a sudden restart, and attempt to repair the storage if it was.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l internal-wipe -d 'Whether CRI-O should wipe containers after a reboot and images after an upgrade when the server starts. If set to false, one must run \'crio wipe\' to wipe the containers and images in these situations. This option is deprecated, and will be removed in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l irqbalance-config-file -r -d 'The irqbalance service config file which is used by CRI-O.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l irqbalance-config-restore-file -r -d 'Determines if CRI-O should attempt to restore the irqbalance config at startup with the mask in this file. Use the \'disable\' value to disable the restore flow entirely.'
complete -c crio -n '__fish_crio_no_subcommand' -l listen -r -d 'Path to the CRI-O socket.'
complete -c crio -n '__fish_crio_no_subcommand' -l log -r -d 'Set the log file path where internal debug information is written.'
complete -c crio -n '__fish_crio_no_subcommand' -l log-dir -r -d 'Default log directory where all logs will go unless directly specified by the kubelet.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l log-filter -r -d 'Filter the log messages by the provided regular expression. For example \'request.\*\' filters all gRPC requests.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l log-format -r -d 'Set the format used by logs: \'text\' or \'json\'.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l log-journald -d 'Log to systemd journal (journald) in addition to kubernetes log file.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l log-level -s l -r -d 'Log messages above specified level: trace, debug, info, warn, error, fatal or panic.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l log-size-max -r -d 'Maximum log size in bytes for a container. If it is positive, it must be >= 8192 to match/exceed conmon read buffer. This option is deprecated. The Kubelet flag \'--container-log-max-size\' should be used instead.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l metrics-cert -r -d 'Certificate for the secure metrics endpoint.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l metrics-collectors -r -d 'Enabled metrics collectors.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l metrics-host -r -d 'Host for the metrics endpoint.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l metrics-key -r -d 'Certificate key for the secure metrics endpoint.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l metrics-port -r -d 'Port for the metrics endpoint.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l metrics-socket -r -d 'Socket for the metrics endpoint.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l minimum-mappable-gid -r -d 'Specify the lowest host GID which can be specified in mappings for a pod that will be run as a UID other than 0. This option is deprecated, and will be replaced with Kubernetes user namespace support (KEP-127) in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l minimum-mappable-uid -r -d 'Specify the lowest host UID which can be specified in mappings for a pod that will be run as a UID other than 0. This option is deprecated, and will be replaced with Kubernetes user namespace support (KEP-127) in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l namespaces-dir -r -d 'The directory where the state of the managed namespaces gets tracked. Only used when manage-ns-lifecycle is true.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l no-pivot -d 'If true, the runtime will not use \'pivot_root\', but instead use \'MS_MOVE\'.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l nri-disable-connections -r -d 'Disable connections from externally started NRI plugins. (default: false)'
complete -c crio -n '__fish_crio_no_subcommand' -f -l nri-listen -r -d 'Socket to listen on for externally started NRI plugins to connect to. (default: "/var/run/nri/nri.sock")'
complete -c crio -n '__fish_crio_no_subcommand' -f -l nri-plugin-config-dir -r -d 'Directory to scan for configuration of pre-installed NRI plugins. (default: "/etc/nri/conf.d")'
complete -c crio -n '__fish_crio_no_subcommand' -f -l nri-plugin-dir -r -d 'Directory to scan for pre-installed NRI plugins to start automatically. (default: "/opt/nri/plugins")'
complete -c crio -n '__fish_crio_no_subcommand' -f -l nri-plugin-registration-timeout -r -d 'Timeout for a plugin to register itself with NRI.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l nri-plugin-request-timeout -r -d 'Timeout for a plugin to handle an NRI request.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l pause-command -r -d 'Path to the pause executable in the pause image.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l pause-image -r -d 'Image which contains the pause executable.'
complete -c crio -n '__fish_crio_no_subcommand' -l pause-image-auth-file -r -d 'Path to a config file containing credentials for --pause-image.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l pids-limit -r -d 'Maximum number of processes allowed in a container. This option is deprecated. The Kubelet flag \'--pod-pids-limit\' should be used instead.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l pinned-images -r -d 'A list of images that will be excluded from the kubelet\'s garbage collection.'
complete -c crio -n '__fish_crio_no_subcommand' -l pinns-path -r -d 'The path to find the pinns binary, which is needed to manage namespace lifecycle. Will be searched for in $PATH if empty.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l profile -d 'Enable pprof remote profiler on 127.0.0.1:6060.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l profile-cpu -r -d 'Write a pprof CPU profile to the provided path.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l profile-mem -r -d 'Write a pprof memory profile to the provided path.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l profile-port -r -d 'Port for the pprof profiler.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l rdt-config-file -r -d 'Path to the RDT configuration file for configuring the resctrl pseudo-filesystem.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l read-only -d 'Setup all unprivileged containers to run as read-only. Automatically mounts the containers\' tmpfs on \'/run\', \'/tmp\' and \'/var/tmp\'.'
complete -c crio -n '__fish_crio_no_subcommand' -l root -s r -r -d 'The CRI-O root directory.'
complete -c crio -n '__fish_crio_no_subcommand' -l runroot -r -d 'The CRI-O state directory.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l runtimes -r -d 'OCI runtimes, format is \'runtime_name:runtime_path:runtime_root:runtime_type:privileged_without_host_devices:runtime_config_path:container_min_memory\'.'
complete -c crio -n '__fish_crio_no_subcommand' -l seccomp-profile -r -d 'Path to the seccomp.json profile to be used as the runtime\'s default. If not specified, then the internal default seccomp profile will be used.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l selinux -d 'Enable selinux support. This option is deprecated, and be interpreted from whether SELinux is enabled on the host in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l separate-pull-cgroup -r -d '[EXPERIMENTAL] Pull in new cgroup.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l shared-cpuset -r -d 'CPUs set that will be used for guaranteed containers that want access to shared cpus'
complete -c crio -n '__fish_crio_no_subcommand' -l signature-policy -r -d 'Path to signature policy JSON file.'
complete -c crio -n '__fish_crio_no_subcommand' -l signature-policy-dir -r -d 'Path to the root directory for namespaced signature policies. Must be an absolute path.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l stats-collection-period -r -d 'The number of seconds between collecting pod and container stats. If set to 0, the stats are collected on-demand instead. DEPRECATED: This option will be removed in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l storage-driver -s s -r -d 'OCI storage driver.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l storage-opt -r -d 'OCI storage driver option.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l stream-address -r -d 'Bind address for streaming socket.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l stream-enable-tls -d 'Enable encrypted TLS transport of the stream server.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l stream-idle-timeout -r -d 'Length of time until open streams terminate due to lack of activity.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l stream-port -r -d 'Bind port for streaming socket. If the port is set to \'0\', then CRI-O will allocate a random free port number.'
complete -c crio -n '__fish_crio_no_subcommand' -l stream-tls-ca -r -d 'Path to the x509 CA(s) file used to verify and authenticate client communication with the encrypted stream. This file can change and CRI-O will automatically pick up the changes.'
complete -c crio -n '__fish_crio_no_subcommand' -l stream-tls-cert -r -d 'Path to the x509 certificate file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes.'
complete -c crio -n '__fish_crio_no_subcommand' -l stream-tls-key -r -d 'Path to the key file used to serve the encrypted stream. This file can change and CRI-O will automatically pick up the changes.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l timezone -s tz -r -d 'To set the timezone for a container in CRI-O. If an empty string is provided, CRI-O retains its default behavior. Use \'Local\' to match the timezone of the host machine.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l tracing-endpoint -r -d 'Address on which the gRPC tracing collector will listen.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l tracing-sampling-rate-per-million -r -d 'Number of samples to collect per million OpenTelemetry spans. Set to 1000000 to always sample.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l uid-mappings -r -d 'Specify the UID mappings to use for the user namespace. This option is deprecated, and will be replaced with Kubernetes user namespace support (KEP-127) in the future.'
complete -c crio -n '__fish_crio_no_subcommand' -l version-file -r -d 'Location for CRI-O to lay down the temporary version file. It is used to check if crio wipe should wipe containers, which should always happen on a node reboot.'
complete -c crio -n '__fish_crio_no_subcommand' -l version-file-persist -r -d 'Location for CRI-O to lay down the persistent version file. It is used to check if crio wipe should wipe images, which should only happen when CRI-O has been upgraded.'
complete -c crio -n '__fish_crio_no_subcommand' -f -l help -s h -d 'show help'
complete -c crio -n '__fish_crio_no_subcommand' -f -l version -s v -d 'print the version'
complete -c crio -n '__fish_crio_no_subcommand' -f -l help -s h -d 'show help'
complete -c crio -n '__fish_crio_no_subcommand' -f -l version -s v -d 'print the version'
complete -c crio -n '__fish_seen_subcommand_from check' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_crio_no_subcommand' -a 'check' -d 'Check CRI-O storage directory for errors.

This command can also repair damaged containers, images and layers.

By default, the data integrity of the storage directory is verified,
which can be an I/O and CPU-intensive operation. The --quick option
can be used to reduce the number of checks run.

When using the --repair option, especially with the --force option,
CRI-O and any currently running containers should be stopped if
possible to ensure no concurrent access to the storage directory
occurs.

The --wipe option can be used to automatically attempt to remove
containers and images on a repair failure. This option, combined
with the --force option, can be used to entirely remove the storage
directory content in case of irrecoverable errors. This should be
used as a last resort, and similarly to the --repair option, it\'s
best if CRI-O and any currently running containers are stopped.'
complete -c crio -n '__fish_seen_subcommand_from check' -f -l age -s a -r -d 'Maximum allowed age for unreferenced layers'
complete -c crio -n '__fish_seen_subcommand_from check' -f -l force -s f -d 'Remove damaged containers'
complete -c crio -n '__fish_seen_subcommand_from check' -f -l repair -s r -d 'Remove damaged images and layers'
complete -c crio -n '__fish_seen_subcommand_from check' -f -l quick -s q -d 'Perform only quick checks'
complete -c crio -n '__fish_seen_subcommand_from check' -f -l wipe -s w -d 'Wipe storage directory on repair failure'
complete -c crio -n '__fish_seen_subcommand_from complete completion' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_crio_no_subcommand' -a 'complete completion' -d 'Generate bash, fish or zsh completions.'
complete -c crio -n '__fish_seen_subcommand_from complete completion' -f -l help -s h -d 'show help'
complete -c crio -n '__fish_seen_subcommand_from help h' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_seen_subcommand_from complete completion' -a 'help h' -d 'Shows a list of commands or help for one command'
complete -c crio -n '__fish_seen_subcommand_from config' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_crio_no_subcommand' -a 'config' -d 'Outputs a commented version of the configuration file that could be used
by CRI-O. This allows you to save you current configuration setup and then load
it later with **--config**. Global options will modify the output.'
complete -c crio -n '__fish_seen_subcommand_from config' -f -l default -d 'Output the default configuration (without taking into account any configuration options).'
complete -c crio -n '__fish_seen_subcommand_from man' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_crio_no_subcommand' -a 'man' -d 'Generate the man page documentation.'
complete -c crio -n '__fish_seen_subcommand_from markdown md' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_crio_no_subcommand' -a 'markdown md' -d 'Generate the markdown documentation.'
complete -c crio -n '__fish_seen_subcommand_from status' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_crio_no_subcommand' -a 'status' -d 'Display status information'
complete -c crio -n '__fish_seen_subcommand_from status' -l socket -s s -r -d 'absolute path to the unix socket'
complete -c crio -n '__fish_seen_subcommand_from config c' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_seen_subcommand_from status' -a 'config c' -d 'Show the configuration of CRI-O as a TOML string.'
complete -c crio -n '__fish_seen_subcommand_from containers container cs s' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_seen_subcommand_from status' -a 'containers container cs s' -d 'Display detailed information about the provided container ID.'
complete -c crio -n '__fish_seen_subcommand_from containers container cs s' -f -l id -s i -r -d 'the container ID'
complete -c crio -n '__fish_seen_subcommand_from info i' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_seen_subcommand_from status' -a 'info i' -d 'Retrieve generic information about CRI-O, such as the cgroup and storage driver.'
complete -c crio -n '__fish_seen_subcommand_from goroutines g' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_seen_subcommand_from status' -a 'goroutines g' -d 'Display the goroutine stack.'
complete -c crio -n '__fish_seen_subcommand_from heap hp' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_seen_subcommand_from status' -a 'heap hp' -d 'Write the heap dump to a temp file and print its location on disk.'
complete -c crio -n '__fish_seen_subcommand_from heap hp' -l file -s f -r -d 'Output file of the heap dump.'
complete -c crio -n '__fish_seen_subcommand_from version' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_crio_no_subcommand' -a 'version' -d 'display detailed version information'
complete -c crio -n '__fish_seen_subcommand_from version' -f -l json -s j -d 'print JSON instead of text'
complete -c crio -n '__fish_seen_subcommand_from version' -f -l verbose -s v -d 'print verbose information (for example all golang dependencies)'
complete -c crio -n '__fish_seen_subcommand_from wipe' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_crio_no_subcommand' -a 'wipe' -d 'wipe CRI-O\'s container and image storage'
complete -c crio -n '__fish_seen_subcommand_from wipe' -f -l force -s f -d 'force wipe by skipping the version check'
complete -c crio -n '__fish_seen_subcommand_from help h' -f -l help -s h -d 'show help'
complete -r -c crio -n '__fish_crio_no_subcommand' -a 'help h' -d 'Shows a list of commands or help for one command'
