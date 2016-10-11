% ocid.conf(5) Open Container Initiative Daemon
% Aleksa Sarai
% OCTOBER 2016

# NAME
ocid.conf - Syntax of OCID configuration file

# DESCRIPTION
The OCID configuration file specifies all of the available command-line options
for the ocid(8) program, but in a TOML format that can be more easily modified
and versioned.

# FORMAT
The [TOML format][toml] is used as the encoding of the configuration file.
Every option and subtable listed here is nested under a global "ocid" table.
No bare options are used. The format of TOML can be simplified to:

    [table]
    option = value

    [table.subtable1]
    option = value

    [table.subtable2]
    option = value

## OCID TABLE

The `ocid` table supports the following options:


**container_dir**=""
  OCID container dir (default: "/var/lib/ocid/containers")

**root**=""
  OCID root dir (default: "/var/lib/ocid")

**sandbox_dir**=""
  OCID pod sandbox dir (default: "/var/lib/ocid/sandboxes")


## OCID.API TABLE

**listen**=""
  Path to ocid socket (default: "/var/run/ocid.sock")

## OCID.RUNTIME TABLE

**conmon**=""
  path to the conmon executable (default: "/usr/libexec/ocid/conmon")

**runtime**=""
  OCI runtime path (default: "/usr/bin/runc")

**selinux**
  Enable selinux support (default: false)

## OCID.IMAGE TABLE

**pause**=""
  Path to the pause executable (default: "/usr/libexec/ocid/pause")

# SEE ALSO
ocid(8)

# HISTORY
Oct 2016, Originally compiled by Aleksa Sarai <asarai@suse.de>
