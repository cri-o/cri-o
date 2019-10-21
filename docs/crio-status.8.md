% crio-status(8) A tool for CRI-O status retrieval
% Sascha Grunert
% JULY 2019

# NAME

crio-status - A tool for CRI-O status retrieval

# SYNOPSIS

crio-status

```
[--socket=[value]]
[--version|-v]
[--help|-h]
```

# DESCRIPTION

The tool `crio-status` can be used to access the provided HTTP API with a
dedicated command line tool.

**Usage**:

```
crio [GLOBAL OPTIONS] command [COMMAND OPTIONS] [ARGUMENTS...]
```

# GLOBAL OPTIONS

**--socket, -s**="": absolute path to the unix socket (default: "/var/run/crio/crio.sock")
**--help, -h**: show help
**--version, -v**: print the version

# COMMANDS

## config, c

Show the configuration of CRI-O as TOML string.

## info, i

Retrieve generic information about CRI-O, like the cgroup and storage driver.

## containers, container, cs, s

Display detailed information about the provided container ID.

**--id, -i**="": the container ID

## complete, completion

Generate bash, fish or zsh completions.

# HISTORY

Jul 2019, Initial version by Sascha Grunert <sgrunert@suse.com>
