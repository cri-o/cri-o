#!/usr/bin/env bats

load helpers

IMAGE="alpine:latest"

function teardown() {
  cleanup_test
}

@test "kpod pull from docker with tag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull debian:6.0.10
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi debian:6.0.10
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from docker without tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull debian
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi debian
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from a non-docker registry with tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull registry.fedoraproject.org/fedora:rawhide
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi registry.fedoraproject.org/fedora:rawhide
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from a non-docker registry without tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull registry.fedoraproject.org/fedora
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi registry.fedoraproject.org/fedora
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull using digest" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull alpine@sha256:1072e499f3f655a032e88542330cf75b02e7bdf673278f701d7ba61629ee3ebe
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi alpine:latest
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from a non existent image" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull umohnani/get-started
	echo "$output"
	[ "$status" -ne 0 ]
}

@test "kpod pull from docker with shortname" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull debian
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi docker.io/debian:latest
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from docker with shortname and tag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull debian:6.0.10
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi docker.io/debian:6.0.10
	echo "$output"
	[ "$status" -eq 0 ]
}
