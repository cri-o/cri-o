#!/usr/bin/env bats

# this test suite tests crio-wipe running with combinations of cri-o and
# podman.

INTEGRATION_ROOT=$BATS_TEST_DIRNAME/../../test
load $INTEGRATION_ROOT/helpers.bash
load test-lib
load lib

PODMAN_BINARY=${PODMAN_BINARY:-$(command -v podman)}

function setup() {
	CONTAINERS_STORAGE_DIR="$TESTDIR/crio"
	crio_wipe::test::prepare "crio version 1.1.1"
}

function teardown() {
	cleanup_test
	if [ ! -z ${PODMAN_BINARY+x} ]; then
		crio_wipe::test::run_podman_with_args stop -a
		crio_wipe::test::run_podman_with_args rm -fa
	fi
}

function remove_func() {
	$BATS_TEST_DIRNAME/crio-wiper --config "$CRIO_CONFIG"
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

	# TODO FIXME, we fail on this check because crio-wipe only wipes
	# if crio has a corresponding container to the image.
	# run crictl images -v
	# [ "$status" -eq 0 ]
	# [ "$output" == "" ]
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

	crio_wipe::test::run_crio_wipe

	start_crio_no_setup
	test_crio_wiped
}


@test "don't clear podman containers" {
	if [ -z ${PODMAN_BINARY+x} ]; then
		skip "Podman not installed"
	fi

	start_crio_with_stopped_pod
	stop_crio_no_clean

	crio_wipe::test::run_podman_with_args run --name test -d quay.io/crio/busybox:latest top

	crio_wipe::test::run_crio_wipe

	crio_wipe::test::run_podman_with_args ps -a
	[[ "$output" =~ "test" ]]
}
