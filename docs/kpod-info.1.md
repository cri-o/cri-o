% kpod(8) # kpod-version - Simple tool to view version information
% Vincent Batts
% JULY 2017

# NAME
kpod-info - Display System Information


# SYNOPSIS
**kpod** **info** [*options* [...]]


# DESCRIPTION

Information display here pertain to the host, current storage stats, and build of kpod. Useful for the user and when reporting issues.


## OPTIONS

**--debug, -D**

Show additional information

**--debug, -D**

Show additional information


## EXAMPLE

`kpod info`

`kpod info --debug --json | jq .host.kernel`

# SEE ALSO
crio(8), crio.conf(5)
