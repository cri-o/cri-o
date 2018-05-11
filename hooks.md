# OCI Hooks Configuration

For POSIX platforms, the [OCI runtime configuration][runtime-spec] supports [hooks][spec-hooks] for configuring custom actions related to the life cycle of the container.
The way you enable the hooks above is by editing the OCI runtime configuration before running the OCI runtime (e.g. [`runc`][runc]).
CRI-O and `podman create` create the OCI configuration for you, and this documentation allows developers to configure CRI-O to set their intended hooks.

One problem with hooks is that the runtime actually stalls execution of the container before running the hooks and stalls completion of the container, until all hooks complete.  This can cause some performance issues.  Also a lot of hooks just check if certain configuration is set and then exit early, without doing anything.  For example the [oci-systemd-hook](https://github.com/projectatomic/oci-systemd-hook) only executes if the command is `init` or `systemd`, otherwise it just exits.  This means if we automatically enabled all hooks, every container would have to execute `oci-systemd-hook`, even if they don't run systemd inside of the container.   Performance would also suffer if we exectuted each hook at each stage ([pre-start][], [post-start][], and [post-stop][]).

## Notational Conventions

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "NOT RECOMMENDED", "MAY", and "OPTIONAL" are to be interpreted as described in [RFC 2119][rfc2119].

## JSON Definition

CRI-O reads all [JSON][] files in `/usr/share/containers/oci/hooks.d/*.json` and `/etc/containers/oci/hooks.d/*.json` to load hook configuration.
If the same file is in both directories, the one in `/etc/containers/oci/hooks.d` takes precedence.

CRI-O watches both hook directories for file creation, writes, and removals, so you can adjust the hooks there without restarting `crio`.

Each JSON file should contain an object with the following properties:

* **`hook`** (REQUIRED, string) Sets [`path`][spec-hooks] in the injected hook.
* **`arguments`** (OPTIONAL, array of strings) Additional arguments to pass to the hook.
    The injected hook's [`args`][spec-hooks] is `hook` with `arguments` appended.
* **`stages`** (REQUIRED, array of strings) Stages when the hook MUST be injected.
    Entries MUST be chosen from:
    * **`prestart`**, to inject [pre-start][].
    * **`poststart`**, to inject [post-start][].
    * **`poststop`**, to inject [post-stop][].
* **`cmds`** (OPTIONAL, array of strings) The hook MUST be injected if the configured [`process.args[0]`][spec-process] matches an entry.
    Entries MUST be [POSIX extended regular expressions][POSIX-ERE].
* **`annotations`** (OPTIONAL, array of strings) The hook MUST be injected if the configured [`annotations`][spec-annotations] matches an entry.
    Entries MUST be [POSIX extended regular expressions][POSIX-ERE].
* **`hasbindmounts`** (OPTIONAL, boolean) The hook MUST be injected if `hasbindmounts` is true and the container is configured to bind-mount host directories into the container.

The matching properties (`cmds`, `annotations` and `hasbindmounts`) are orthogonal, and the hook is injected if *any* of those properties match.

## Example

The following configuration tells CRI-O to inject `oci-systemd-hook` in the [pre-start][] and [post-stop][] stages if [`process.args[0]`][spec-process] ends with `/init` or `/systemd`:

```console
$ cat /etc/containers/oci/hooks.d/oci-systemd-hook.json
{
    "cmds": [".*/init$" , ".*/systemd$" ],
    "hook": "/usr/libexec/oci/hooks.d/oci-systemd-hook",
    "stages": [ "prestart", "poststop" ]
}
```

The following example tells CRI-O to inject `oci-umount --debug` in the [pre-start][] phase if the container is configured to bind-mount host directories into the container.

```console
cat /etc/containers/oci/hooks.d/oci-systemd-hook.json
{
    "hasbindmounts": true,
    "hook": "/usr/libexec/oci/hooks.d/oci-umount",
    "stages": [ "prestart" ]
    "arguments": [ "--debug" ]
}
```

[JSON]: https://tools.ietf.org/html/rfc8259
[POSIX-ERE]: http://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap09.html#tag_09_04
[post-start]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#poststart
[post-stop]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#poststop
[pre-start]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#prestart
[rfc2119]: http://tools.ietf.org/html/rfc2119
[runc]: https://github.com/opencontainers/runc
[runtime-spec]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/spec.md
[spec-annotations]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#annotations
[spec-hooks]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#posix-platform-hooks
[spec-process]: https://github.com/opencontainers/runtime-spec/blob/v1.0.1/config.md#process
