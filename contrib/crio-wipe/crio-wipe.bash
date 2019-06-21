#!/bin/bash

set -eu


dir=${0%/*}
source "$dir/lib.bash"

VERSION_FILE_LOCATION="/var/lib/crio/version"
CONTAINERS_STORAGE_DIR="/var/lib/containers"
NEW_VERSION=$(crio --version)
WIPE=0

print_usage() {
	echo "$(basename $0) [-f version-file-location] [-d containers-storage-dir] [-w wipe]"
}

# process command variables
while getopts 'f:d:w:h' OPTION; do
	case "$OPTION" in
		f) VERSION_FILE_LOCATION="$OPTARG" ;;
		d) CONTAINERS_STORAGE_DIR="$OPTARG" ;;
		w)
		# We need to make sure arguments to -w are integers.
		# the way to check this is verifying it can be compared with -eq,
		# which only accepts numerical arguments
		if [ -n "$OPTARG" ] && [ "$OPTARG" -eq "$OPTARG" ] 2>/dev/null; then
			WIPE=$OPTARG
		else
			>&2 echo "argument to -w needs to be an integer"
			exit 1
		fi
			;;
		h) print_usage; exit 0;;
		?) >&2 print_usage; exit 1;;
	esac
done
shift "$(($OPTIND -1))"

main
