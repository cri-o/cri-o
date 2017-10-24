#!/usr/bin/env bats

load helpers

IMAGE="redis:alpine"

function teardown() {
	cleanup_test
}

@test "bind secrets mounts to container" {
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
    run crioctl ctr execsync --id "$ctr_id" cat /proc/mounts
    echo "$output"
    [ "$status" -eq 0 ]
    mount_info="$output"
    run grep /container/path1 <<< "$mount_info"
    echo "$output"
    [ "$status" -eq 0 ]
    rm -rf ${MOUNT_PATH}
    cleanup_ctrs
    cleanup_pods
    stop_crio
}
