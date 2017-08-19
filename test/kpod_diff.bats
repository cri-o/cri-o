#/usr/bin/env bats

load helpers

IMAGE="alpine:latest"
ROOT="$TESTDIR/crio"
RUNROOT="$TESTDIR/crio-run"
KPOD_OPTIONS="--root $ROOT --runroot $RUNROOT --storage-driver vfs"

function teardown() {
    cleanup_test
}

@test "test diff of image and parent" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull $IMAGE
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} $KPOD_OPTIONS diff $IMAGE
    [ "$status" -eq 0 ]
    echo "$output"
    run ${KKPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
}

@test "test diff on non-existent layer" {
    run ${KPOD_BINARY} $KPOD_OPTIONS diff "abc123"
    [ "$status" -ne 0 ]
    echo "$output"
}

@test "test diff with json output" {
    run ${KPOD_BINARY} $KPOD_OPTIONS pull $IMAGE
    [ "$status" -eq 0 ]
    # run bash -c "${KPOD_BINARY} ${KPOD_OPTIONS} diff --format json $IMAGE | python -m json.tool"
    run ${KPOD_BINARY} $KPOD_OPTIONS diff --format json $IMAGE
    [ "$status" -eq 0 ]
    echo "$output"
    run ${KKPOD_BINARY} $KPOD_OPTIONS rmi $IMAGE
}
