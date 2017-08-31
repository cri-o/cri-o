#!/usr/bin/env bats

load helpers

IMAGE="alpine:latest"
ROOT="$TESTDIR/crio"
RUNROOT="$TESTDIR/crio-run"
KPOD_OPTIONS="--root $ROOT --runroot $RUNROOT --storage-driver vfs"

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
    run kpod rmi "$IMAGE" busybox:test
    echo "$output"
    [ "$status" -eq 0 ]
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
    rm -rf /tmp/busybox
    run kpod rmi "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
}

@test "kpod push to docker archive" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push "$IMAGE" docker-archive:/tmp/busybox-archive:1.26
    echo "$output"
    [ "$status" -eq 0 ]
    rm /tmp/busybox-archive
    run kpod rmi "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
}

@test "kpod push to oci without compression" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
    run mkdir /tmp/oci-busybox
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS push "$IMAGE" oci:/tmp/oci-busybox
    echo "$output"
    [ "$status" -eq 0 ]
    rm -rf /tmp/oci-busybox
    run kpod rmi "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
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
    run kpod rmi "$IMAGE"
    echo "$output"
    [ "$status" -eq 0 ]
}
