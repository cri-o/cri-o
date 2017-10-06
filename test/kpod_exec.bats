#!/usr/bin/env bats

load helpers

IMAGE="redis:alpine"

function teardown() {
    cleanup_test
}

@test "exec on a bogus container" {
    run ${KPOD_BINARY} ${KPOD_OPTIONS} exec foobar ls
    echo "$output"
    [ "$status" -eq 1 ]
}

@test "Run a simple command" {
    start_crio
    run crioctl pod run --config "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"
    run crioctl image pull "$IMAGE"
    [ "$status" -eq 0 ]
    run crioctl ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
    echo "$output"
    [ "$status" -eq 0 ]
    ctr_id="$output"
    run crioctl ctr start --id "$ctr_id"
    echo "$output"
    run ${KPOD_BINARY} ${KPOD_OPTIONS} exec "$ctr_id" ls
    echo "$output"
    [ "$status" -eq 0 ]
    cleanup_pods
    stop_crio
}

# Disabled until runc is fixed upstream to handle
# longer and more complicated command sequences.
#@test "Check for environment variable" {
#    start_crio
#    run crioctl pod run --config "$TESTDATA"/sandbox_config.json
#    echo "$output"
#    [ "$status" -eq 0 ]
#    pod_id="$output"
#    run crioctl image pull "$IMAGE"
#    [ "$status" -eq 0 ]
#    run crioctl ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
#    echo "$output"
#    [ "$status" -eq 0 ]
#    ctr_id="$output"
#    run crioctl ctr start --id "$ctr_id"
#    echo "$output"
#    run ${KPOD_BINARY} ${KPOD_OPTIONS} exec -e FOO=bar "$ctr_id" env | grep FOO
#    echo "$output"
#    [ "$status" -eq 0 ]
#    cleanup_pods
#    stop_crio
#}

@test "Execute command in a non-running container" {
    start_crio
    run crioctl pod run --config "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"
    run crioctl image pull "$IMAGE"
    [ "$status" -eq 0 ]
    run crioctl ctr create --config "$TESTDATA"/container_config.json --pod "$pod_id"
    echo "$output"
    [ "$status" -eq 0 ]
    ctr_id="$output"
    run ${KPOD_BINARY} ${KPOD_OPTIONS} exec ls
    echo "$output"
    [ "$status" -eq 1 ]
    cleanup_pods
    stop_crio
}
