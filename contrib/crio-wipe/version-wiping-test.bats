#!/usr/bin/env bats

# This test suite tests crio-wipe runs when expected
# based on the found version file and crio version
# it uses rm -rf as a remove func, as it's easier to test
# a wipe happened

load test-lib
load lib

function remove_func() {
	rm -rf "$CONTAINERS_STORAGE_DIR"
}

function setup() {
	CONTAINERS_STORAGE_DIR="$TESTDIR/crio"
}

function teardown() {
	crio_wipe::test::cleanup
}

@test "don't upgrade if not told to wipe" {
	crio_wipe::test::prepare "crio version 1.1.1"
	WIPE=1
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ -d "$TMP_STORAGE" ]
}

@test "upgrade with no version file" {
	crio_wipe::test::prepare "crio version 1.1.1"
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}

@test "don't upgrade with same version" {
	crio_wipe::test::prepare "crio version 1.13.1" "\"1.13.1\""
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ -d "$TMP_STORAGE" ]
}

@test "don't upgrade for sub-minor releases" {
	crio_wipe::test::prepare "crio version 1.13.2" "\"1.13.1\""
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ -d "$TMP_STORAGE" ]
}

@test "upgrade for minor releases" {
	crio_wipe::test::prepare "crio version 1.14.0" "\"1.13.1\""
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}

@test "upgrade for major release" {
	crio_wipe::test::prepare "crio version 2.0.0" "\"1.13.1\""
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}

@test "fail and not upgrade with bad version format" {
	crio_wipe::test::prepare "crio version bad format" "\"1.13.1\""
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 1 ]
	[ -d "$TMP_STORAGE" ]
}

@test "fail and not upgrade with bad minor version format" {
	crio_wipe::test::prepare "crio version 1.bad minor format.11" "\"1.13.1\""
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 1 ]
	[ -d "$TMP_STORAGE" ]
}

@test "fail and upgrade with faulty version file" {
	crio_wipe::test::prepare "crio version 1.1.1" "bad format"
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}

@test "fail and upgrade with faulty minor version in version file" {
	crio_wipe::test::prepare "crio version 1.14.11" "\"1.x.14\""
	run crio_wipe::main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}
