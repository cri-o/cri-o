#!/usr/bin/env bats

load helpers

IMAGE="alpine:latest"
ROOT="$TESTDIR/crio"
RUNROOT="$TESTDIR/crio-run"
KPOD_OPTIONS="--root $ROOT --runroot $RUNROOT $STORAGE_OPTS"

function teardown() {
    cleanup_test
}

@test "kpod push to containers/storage" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push "$IMAGE" containers-storage:[$ROOT]busybox:test
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi "$IMAGE"
    [ "$status" -eq 0 ]
    stop_crio
}

@test "kpod push to directory" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run mkdir /tmp/busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push "$IMAGE" dir:/tmp/busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi "$IMAGE"
    [ "$status" -eq 0 ]
    rm -rf /tmp/busybox
    stop_crio
}

@test "kpod push to docker archive" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push "$IMAGE" docker-archive:/tmp/busybox-archive:1.26
    echo "$output"
    [ "$status" -eq 0 ]
    rm /tmp/busybox-archive
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi "$IMAGE"
    [ "$status" -eq 0 ]
    stop_crio
}

@test "kpod push to oci without compression" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run mkdir /tmp/oci-busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push --disable-compression "$IMAGE" oci:/tmp/oci-busybox
    echo "$output"
    [ "$status" -eq 0 ]
    rm -rf /tmp/oci-busybox
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi "$IMAGE"
    [ "$status" -eq 0 ]
    stop_crio
}

@test "kpod push without signatures" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run mkdir /tmp/busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push --remove-signatures "$IMAGE" dir:/tmp/busybox
    echo "$output"
    [ "$status" -eq 0 ]
    rm -rf /tmp/busybox
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi "$IMAGE"
    [ "$status" -eq 0 ]
    stop_crio
}
