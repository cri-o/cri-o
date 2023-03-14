#!/usr/bin/env bash

# In helpers.bats, the "runtime()" wrapper is used to call the runtime
# binary directly. It is used for 4 commands across the test files:
#
# list:
#   Provides the list of containers running under the runtime.
#   In this implementation, the output comes from a call to "ps" and contains
#   the PID and command line used to run the kata process.
#   with the "-q" parameter, the list is limited to the container ID of running
#   containers.
#
# kill:
# delete:
# state:
#
# As there is no cmdline tool for kata, we have to find a different way of
# retrieving the same information.
# This script is used to mimic this behaviour and return meaningful information
# in the kata use case.
#

case $1 in
"list")
    LIST=$(ps -o pid= -o args= -C containerd-shim)
    if [ "$2" == "-q" ]; then
        # list only the container IDs
        echo "$LIST" | grep -oP '(?<=-id )[^ ]*'
    else
        echo "$LIST"
    fi
    ;;

"kill" | "delete" | "state")
    echo "runtime $1 - not implemented in test script"
    exit 1
    ;;

*)
    echo "Not supported command: $1"
    exit 1
    ;;

esac
