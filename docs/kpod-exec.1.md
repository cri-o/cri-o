% kpod(1) kpod-exec - Run a command in a running container
% Brent Baude
# kpod-exec "1" "October 2017" "kpod"

## NAME
kpod exec - Run a command in a running container

## SYNOPSIS
**kpod exec [OPTIONS] CONTAINER COMMAND

## DESCRIPTION
Run a command in a running container. The command will only run while the container's primary process (PID 1) is running.
restarted if the container is restarted.

## OPTIONS

**--detach, -d**

Detached mode: run the command in the background.

**--env, -e**

Set one or environment variables for the container environment.

**--tty, -t**

Allocate a pseudo-TTY.

**--user, -u**

Sets the username or UID used and optionally the groupname or GID for the specified command. The format is *uid:gid*.


## EXAMPLE

kpod exec mywebserver ls

kpod exec 860a4b23 ls

kpod exec -e FOO=bar -e BAR=foo 860a4b23 ls

kpod exec -u foo:users mywebserver ls

## SEE ALSO
kpod(1)

## HISTORY
October 2017, Originally compiled by Brent Baude <bbaude@redhat.com>
