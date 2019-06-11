#!/bin/bash

set -eu

prepare_test() {
	NEW_VERSION="$1"
	WIPE=0
	TESTDIR=$(mktemp -d)
	# create and set our version file
	VERSION_FILE_LOCATION="$TESTDIR"/version.tmp
	if [[ ! -z ${2+x} ]]; then
		echo "$2" > "$VERSION_FILE_LOCATION"
	fi


	# setup storage dir
	TMP_STORAGE="$TESTDIR/tmp"
	mkdir "$TMP_STORAGE"
	CONTAINERS_STORAGE_DIR="$TMP_STORAGE"
}

cleanup_test() {
	rm -rf "$TESTDIR"
}
