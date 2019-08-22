#!/bin/bash

set -eu

crio_wipe::test::prepare() {
	NEW_VERSION="$1"
	WIPE=0
	TESTDIR=${TESTDIR:-$(mktemp -d)}
	# create and set our version file
	VERSION_FILE_LOCATION="$TESTDIR"/version.tmp
	if [[ ! -z ${2+x} ]]; then
		echo "$2" > "$VERSION_FILE_LOCATION"
	fi

	if [[ ! -z ${CONTAINERS_STORAGE_DIR+x} ]]; then
		# setup storage dir if not specified by caller
		TMP_STORAGE="$TESTDIR/tmp"
		mkdir "$TMP_STORAGE"
		CONTAINERS_STORAGE_DIR="$TMP_STORAGE"
	fi
}

crio_wipe::test::cleanup() {
	rm -rf "$TESTDIR"
}

function crio_wipe::test::run_podman_with_args() {
	run "$PODMAN_BINARY" --root "$TESTDIR/crio" --runroot "$TESTDIR/crio-run" "$@"
	echo "$output"
	[ "$status" -eq 0 ]
}

# run crio_wipe calls crio_wipe and tests it succeeded
function crio_wipe::test::run_crio_wipe() {
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" -eq 0 ]
}
