% crio-status(8) A tool for CRI-O status retrieval

% Sascha Grunert

# NAME

crio-status - A tool for CRI-O status retrieval

# SYNOPSIS

crio-status

```
[--help|-h]
[--socket|-s]=[value]
[--version|-v]
```

**Usage**:

```
crio-status [GLOBAL OPTIONS] command [COMMAND OPTIONS] [ARGUMENTS...]
```

# GLOBAL OPTIONS

**--help, -h**: show help

**--socket, -s**="": absolute path to the unix socket (default: "/var/run/crio/crio.sock")

**--version, -v**: print the version


# COMMANDS

## complete, completion

Output shell completion code

## config, c

Show the configuration of CRI-O as TOML string.

**--socket, -s**="": absolute path to the unix socket (default: "/var/run/crio/crio.sock")

## containers, container, cs, s

Display detailed information about the provided container ID.

**--id, -i**="": the container ID

**--socket, -s**="": absolute path to the unix socket (default: "/var/run/crio/crio.sock")

## info, i

Retrieve generic information about CRI-O, like the cgroup and storage driver.

**--socket, -s**="": absolute path to the unix socket (default: "/var/run/crio/crio.sock")

## man

Generate the man page documentation.

## markdown, md

Generate the markdown documentation.

## help, h

Shows a list of commands or help for one command