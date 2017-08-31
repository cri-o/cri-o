#!/usr/bin/env bats

load helpers

IMAGE="redis:alpine"
ROOT="$TESTDIR/crio"
RUNROOT="$TESTDIR/crio-run"
KPOD_OPTIONS="--root $ROOT --runroot $RUNROOT --storage-driver vfs"

function teardown() {
    cleanup_test
}

@test "kpod inspect image" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull ${IMAGE}
    echo "$output"
    [ "$status" -eq 0 ]
    run bash -c "${KPOD_BINARY} $KPOD_OPTIONS inspect ${IMAGE} | python -m json.tool"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi ${IMAGE}
    echo "$output"
    [ "$status" -eq 0 ]
}


@test "kpod inspect non-existent container" {
    run ${KPOD_BINARY} $KPOD_OPTIONS inspect 14rcole/non-existent
    echo "$output"
    [ "$status" -ne 0 ]
}

@test "kpod inspect with format" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull ${IMAGE}
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS inspect --format {{.ID}} ${IMAGE}
    echo "$output"
    [ "$status" -eq 0 ]
    inspectOutput="$output"
    run ${KPOD_BINARY} $KPOD_OPTIONS images --quiet ${IMAGE}
    echo "$output"
    [ "$status" -eq 0 ]
    [ "$output" = "$inspectOutput" ]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi ${IMAGE}
    echo "$output"
    [ "$status" -eq 0 ]
}

@test "kpod inspect specified type" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull ${IMAGE}
    echo "$output"
    [ "$status" -eq 0 ]
    run bash -c "${KPOD_BINARY} $KPOD_OPTIONS inspect --type image ${IMAGE} | python -m json.tool"
    echo "$output"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi ${IMAGE}
    echo "$output"
    [ "$status" -eq 0 ]
}
