#!/usr/bin/env bats

load helpers

ROOT="$TESTDIR/crio"
RUNROOT="$TESTDIR/crio-run"
KPOD_OPTIONS="--root $ROOT --runroot $RUNROOT --storage-driver vfs"

@test "kpod version test" {
	run ${KPOD_BINARY} version
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from docker with tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull debian:6.0.10
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from docker without tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull debian
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from a non-docker registry with tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull registry.fedoraproject.org/fedora:rawhide
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from a non-docker registry without tag" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull registry.fedoraproject.org/fedora
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull using digest" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull debian@sha256:7d067f77d2ae5a23fe6920f8fbc2936c4b0d417e9d01b26372561860750815f0
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "kpod pull from a non existent image" {
	run ${KPOD_BINARY} $KPOD_OPTIONS pull umohnani/get-started
	echo "$output"
	[ "$status" -ne 0 ]
}
