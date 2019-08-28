#!/usr/bin/env bats

# this test suite tests crio-wipe running with combinations of cri-o and
# podman.

INTEGRATION_ROOT=$BATS_TEST_DIRNAME/../../test
load $INTEGRATION_ROOT/helpers.bash
load test-lib

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

@test "clear simple sandbox" {
	crio_wipe::test::start_crio_with_stopped_pod
	stop_crio_no_clean

	crio_wipe::test::run_crio_wipe

	start_crio_no_setup
	crio_wipe::test::test_crio_wiped
}


@test "don't clear podman containers" {
	if [ -z ${PODMAN_BINARY+x} ]; then
		skip "Podman not installed"
	fi

	crio_wipe::test::start_crio_with_stopped_pod
	stop_crio_no_clean

	crio_wipe::test::run_podman_with_args run --name test -d quay.io/crio/busybox:latest top

	crio_wipe::test::run_crio_wipe

	crio_wipe::test::run_podman_with_args ps -a
	[[ "$output" =~ "test" ]]
}
