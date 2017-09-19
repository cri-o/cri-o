#!/usr/bin/env bats

load helpers

ROOT="$TESTDIR/crio"
RUNROOT="$TESTDIR/crio-run"
KPOD_OPTIONS="--root $ROOT --runroot $RUNROOT $STORAGE_OPTS"
function teardown() {
    cleanup_test
}

@test "pause a bogus container" {
    run ${KPOD_BINARY} ${KPOD_OPTIONS} pause foobar
    echo "$output"
    [ "$status" -eq 1 ]
}

@test "unpause a bogus container" {
    run ${KPOD_BINARY} ${KPOD_OPTIONS} unpause foobar
    echo "$output"
    [ "$status" -eq 1 ]
}

@test "pause a running container by id" {
    start_crio
    run crioctl pod run --config "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"
    run crioctl ctr create --config "$TESTDATA"/container_config.json --pod "$pod_id"
    echo "$output"
    [ "$status" -eq 0 ]
    ctr_id="$output"
    run crioctl ctr start --id "$ctr_id"
    echo "$output"
    id="$output"
    run ${KPOD_BINARY} ${KPOD_OPTIONS} pause "$id"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} ${KPOD_OPTIONS} unpause "$id"
    echo "$output"
    [ "$status" -eq 0 ]
    cleanup_pods
    pause_crio
}

@test "pause a running container by name" {
    start_crio
    run crioctl pod run --config "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"
    run crioctl ctr create --config "$TESTDATA"/container_config.json --pod "$pod_id"
    echo "$output"
    [ "$status" -eq 0 ]
    ctr_id="$output"
    run crioctl ctr start --id "$ctr_id"
    echo "$output"
    run ${KPOD_BINARY} ${KPOD_OPTIONS} pause "k8s_podsandbox1-redis_podsandbox1_redhat.test.crio_redhat-test-crio_0"
    echo "$output"
    [ "$status" -eq 0 ]
    run ${KPOD_BINARY} ${KPOD_OPTIONS} unpause "k8s_podsandbox1-redis_podsandbox1_redhat.test.crio_redhat-test-crio_0"
    echo "$output"
    [ "$status" -eq 0 ]
    cleanup_pods
    pause_crio
}
