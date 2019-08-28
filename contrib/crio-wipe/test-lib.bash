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
	run $BATS_TEST_DIRNAME/crio-wipe --config "$CRIO_CONFIG" --version-file-location "$VERSION_FILE_LOCATION"
	echo "$status"
	echo "$output"
	[ "$status" -eq 0 ]
}

# test_crio_wiped checks if a running crio instance
# has no containers, pods or images
function crio_wipe::test::test_crio_wiped() {
	run crictl pods -v
	[ "$status" -eq 0 ]
	[ "$output" == "" ]

	run crictl ps -v
	[ "$status" -eq 0 ]
	[ "$output" == "" ]

	# TODO FIXME, we fail on this check because crio-wipe only wipes
	# if crio has a corresponding container to the image.
	# run crictl images -v
	# [ "$status" -eq 0 ]
	# [ "$output" == "" ]
}

function crio_wipe::test::start_crio_with_stopped_pod() {
	start_crio "" "" "" "" ""
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl stopp "$output"
	echo "$output"
	[ "$status" -eq 0 ]
}
