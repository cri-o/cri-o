#!/usr/bin/env bats

load helpers

IMAGE="alpine:latest"
ROOT="$TESTDIR/crio"
RUNROOT="$TESTDIR/crio-run"
KPOD_OPTIONS="--root $ROOT --runroot $RUNROOT $STORAGE_OPTS"

function teardown() {
    cleanup_test
}

@test "kpod images" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull $IMAGE
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS images
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
    [ "$status" -eq 0 ]
}

@test "kpod images test valid json" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull $IMAGE
    echo "$output"
    [ "$status" -eq 0 ]
    run bash -c "${KPOD_BINARY} $KPOD_OPTIONS images --format json | python -m json.tool"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
    [ "$status" -eq 0 ]
}

@test "kpod images check name json output" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull $IMAGE
    [ "$status" -eq 0 ]
    run bash -c "${KPOD_BINARY} $KPOD_OPTIONS images --format json | python -c 'import sys; import json; print(json.loads(sys.stdin.read()))[0][\"names\"]'"
    echo "$output"
    image=$(echo "$output" | tr -d '\n')
    [ "$image" == "docker.io/library/alpine:latest" ]
    run ${KPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
    [ "$status" -eq 0 ]
}
