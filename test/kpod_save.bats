#!/usr/bin/env bats

load helpers

IMAGE="alpine:latest"
ROOT="$TESTDIR/crio"
RUNROOT="$TESTDIR/crio-run"
KPOD_OPTIONS="--root $ROOT --runroot $RUNROOT $STORAGE_OPTS"

function teardown() {
    cleanup_test
}

@test "kpod save output flag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} save -o alpine.tar $IMAGE
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} rmi $IMAGE
	[ "$status" -eq 0 ]
	rm -f alpine.tar
	[ "$status" -eq 0 ]
}

@test "kpod save using stdout" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} save > alpine.tar $IMAGE
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} rmi $IMAGE
	[ "$status" -eq 0 ]
	rm -f alpine.tar
	[ "$status" -eq 0 ]
}

@test "kpod save quiet flag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} save -q -o alpine.tar $IMAGE
	echo "$output"
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} rmi $IMAGE
	[ "$status" -eq 0 ]
	rm -f alpine.tar
	[ "$status" -eq 0 ]
}

@test "kpod save non-existent image" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} save -o alpine.tar $IMAGE
	echo "$output"
	[ "$status" -ne 0 ]
}
