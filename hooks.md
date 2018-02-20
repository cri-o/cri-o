# OCI Hooks Configuration

For POSIX platforms, the [OCI runtime configuration][runtime-spec] supports [hooks][spec-hooks] for configuring custom actions related to the life cycle of the container.
The way you enable the hooks above is by editing the OCI runtime configuration before running the OCI runtime (e.g. [`runc`][runc]).
CRI-O and `Kpod create` create the OCI configuration for you, and this documentation allows developers to configure CRI-O to set their intended hooks.

One problem with hooks is that the runtime actually stalls execution of the container before running the hooks and stalls completion of the container, until all hooks complete.  This can cause some performance issues.  Also a lot of hooks just check if certain configuration is set and then exit early, without doing anything.  For example the [oci-systemd-hook](https://github.com/projectatomic/oci-systemd-hook) only executes if the command is `init` or `systemd`, otherwise it just exits.  This means if we automatically enable all hooks, every container will have to execute oci-systemd-hook, even if they don't run systemd inside of the container.   Also since there are three stages, prestart, poststart, poststop each hook gets executed three times.



## Json Definition

We decided to add a json file for hook builders which allows them to tell CRI-O when to run the hook and in which stage.
CRI-O reads all json files in /usr/share/containers/oci/hooks.d/*.json and /etc/containers/oci/hooks.d and sets up the specified hooks to run.  If the same name is in both directories, the one in /etc/containers/oci/hooks.d takes precedence.

The json configuration looks like this in GO
```
// HookParams is the structure returned from read the hooks configuration
type HookParams struct {
	Hook          string   `json:"hook"`
	Stage         []string `json:"stages"`
	Cmds          []string `json:"cmds"`
	Annotations   []string `json:"annotations"`
	HasBindMounts bool     `json:"hasbindmounts"`
	Arguments     []string `json:"arguments"`
}
```

| Key    | Description                                                                                                                        | Required/Optional |
| ------ |----------------------------------------------------------------------------------------------------------------------------------- | -------- |
| hook   | Path to the hook                                                                                                                   | Required |
| stages | List of stages to run the hook in: Valid options are `prestart`, `poststart`, `poststop`                                           | Required |
| cmds   | List of regular expressions to match the command for running the container.  If the command matches a regex, the hook will be run  | Optional |
| annotations | List of regular expressions to match against the Annotations in the container runtime spec, if an Annotation matches the hook will be run|optional |
| hasbindmounts | Tells CRI-O to run the hook if the container has bind mounts from the host into the container | Optional |
| arguments | Additional arguments to append to the hook command when executing it. For example --debug | Optional |

## Example


```
cat /etc/containers/oci/hooks.d/oci-systemd-hook.json
{
    "cmds": [".*/init$" , ".*/systemd$" ],
    "hook": "/usr/libexec/oci/hooks.d/oci-systemd-hook",
    "stages": [ "prestart", "poststop" ]
}
```

In the above example CRI-O will only run the oci-systemd-hook in the prestart and poststop stage, if the command ends with /init or /systemd


```
cat /etc/containers/oci/hooks.d/oci-systemd-hook.json
{
    "hasbindmounts": true,
    "hook": "/usr/libexec/oci/hooks.d/oci-umount",
    "stages": [ "prestart" ]
    "arguments": [ "--debug" ]
}
```
In this example the oci-umount will only be run during the prestart phase if the container has volume/bind mounts from the host into the container, it will also execute oci-umount with the --debug argument.

[runc]: https://github.com/opencontainers/runc
[runtime-spec]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/spec.md
[spec-hooks]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#posix-platform-hooks
