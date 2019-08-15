#!/usr/bin/env bats

# this test suite tests crio wipe running with combinations of cri-o and
# podman.

load ../helpers
PODMAN_BINARY=${PODMAN_BINARY:-$(command -v podman)}

function setup() {
	setup_test
	# create and set our version file
	VERSION_FILE_LOCATION="$TESTDIR"/version.tmp
	if [[ ! -z ${1+x} ]]; then
		echo "$1" > "$VERSION_FILE_LOCATION"
	fi
}

function teardown() {
	cleanup_test
	run_podman_with_args stop -a
	run_podman_with_args rm -fa
}

function run_podman_with_args() {
	if [ ! -z ${PODMAN_BINARY+x} ]; then
		run "$PODMAN_BINARY" --root "$TESTDIR/crio" --runroot "$TESTDIR/crio-run" "$@"
		echo "$output"
		[ "$status" -eq 0 ]
	fi
}

# run crio_wipe calls crio_wipe and tests it succeeded
function run_crio_wipe() {
	run $CRIO_BINARY --config "$CRIO_CONFIG" --version-file "$VERSION_FILE_LOCATION" wipe
	echo "$status"
	echo "$output"
	[ "$status" -eq 0 ]
}

# test_crio_wiped checks if a running crio instance
# has no containers, pods or images
function test_crio_wiped() {
	run crictl pods -v
	[ "$status" -eq 0 ]
	[ "$output" == "" ]

	run crictl ps -v
	[ "$status" -eq 0 ]
	[ "$output" == "" ]
}

function start_crio_with_stopped_pod() {
	start_crio "" "" "" "" ""
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl stopp "$output"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "clear simple sandbox" {
	start_crio_with_stopped_pod
	stop_crio_no_clean

	run_crio_wipe

	start_crio_no_setup
	test_crio_wiped
}


@test "don't clear podman containers" {
	if [ -z ${PODMAN_BINARY+x} ]; then
		skip "Podman not installed"
	fi

	start_crio_with_stopped_pod
	stop_crio_no_clean

	run_podman_with_args run --name test -d quay.io/crio/busybox:latest top

	run_crio_wipe

	run_podman_with_args ps -a
	[[ "$output" =~ "test" ]]
}
