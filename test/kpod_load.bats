#!/usr/bin/env bats

load helpers

IMAGE="alpine:latest"

function teardown() {
    cleanup_test
}

@test "kpod load input flag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} save -o alpine.tar $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} load -i alpine.tar
	echo "$output"
	[ "$status" -eq 0 ]
	rm -f alpine.tar
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
}

@test "kpod load oci-archive image" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} save -o alpine.tar --format oci-archive $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} load -i alpine.tar
	echo "$output"
	[ "$status" -eq 0 ]
	rm -f alpine.tar
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
}

@test "kpod load using quiet flag" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} pull $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} save -o alpine.tar $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} ${KPOD_OPTIONS} load -q -i alpine.tar
	echo "$output"
	[ "$status" -eq 0 ]
	rm -f alpine.tar
	[ "$status" -eq 0 ]
	run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
	[ "$status" -eq 0 ]
}

@test "kpod load non-existent file" {
	run ${KPOD_BINARY} ${KPOD_OPTIONS} load -i alpine.tar
	echo "$output"
	[ "$status" -ne 0 ]
}
