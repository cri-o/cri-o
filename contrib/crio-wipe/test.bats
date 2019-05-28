#!/usr/bin/env bats

load lib
load helpers

function teardown() {
	cleanup_test
}

@test "don't upgrade if not told to wipe" {
	prepare_test "crio version 1.1.1"
	WIPE=1
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ -d "$TMP_STORAGE" ]
}

@test "upgrade with no version file" {
	prepare_test "crio version 1.1.1"
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}

@test "don't upgrade with same version" {
	prepare_test "crio version 1.13.1" "\"1.13.1\""
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ -d "$TMP_STORAGE" ]
}

@test "don't upgrade for sub-minor releases" {
	prepare_test "crio version 1.13.2" "\"1.13.1\""
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ -d "$TMP_STORAGE" ]
}

@test "upgrade for minor releases" {
	prepare_test "crio version 1.14.0" "\"1.13.1\""
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}

@test "upgrade for major release" {
	prepare_test "crio version 2.0.0" "\"1.13.1\""
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}

@test "fail and not upgrade with bad version format" {
	prepare_test "crio version bad format" "\"1.13.1\""
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 1 ]
	[ -d "$TMP_STORAGE" ]
}

@test "fail and not upgrade with bad minor version format" {
	prepare_test "crio version 1.bad minor format.11" "\"1.13.1\""
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 1 ]
	[ -d "$TMP_STORAGE" ]
}

@test "fail and upgrade with faulty version file" {
	prepare_test "crio version 1.1.1" "bad format"
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}

@test "fail and upgrade with faulty minor version in version file" {
	prepare_test "crio version 1.14.11" "\"1.x.14\""
	run main
	echo "$status"
	echo "$output"
	[ "$status" == 0 ]
	[ ! -d "$TMP_STORAGE" ]
}
