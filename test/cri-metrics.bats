#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
    setup_test
    PORT="9090"
    CONTAINER_ENABLE_METRICS="true" setup_crio
    cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = [
    "network",
    "cpu",
    "memory",
    "oom",
]
EOF
    start_crio_no_setup
    if ! port_listens "$PORT"; then
        echo "Port $PORT is not listening" 
        exit 1
    fi

}

function teardown() {
    cleanup_test
}

function metrics_setup() {
    # start sandbox
    POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
    # Make sure we get a non-empty metrics response
    crictl metricsp | grep "podSandboxId"
    CONTAINER_ID=$(crictl create $POD_ID "$TESTDATA/container_sleep.json" "$TESTDATA/sandbox_config.json")
    crictl start $CONTAINER_ID
    # assert pod metrics are present
    crictl metricsp | grep "container_network_receive_bytes_total"
    # assert container metrics are present
    crictl metricsp | grep "container_memory_usage_bytes"
}


@test "verify container_memory_usage_bytes" {
    metrics_setup

    set_container_pod_cgroup_root "" "$CONTAINER_ID"

    cmd='for i in {1..10}; do dd if=/dev/zero of=/dev/null bs=10M count=1; done'
    crictl exec --sync "$CONTAINER_ID" /bin/sh -c "$cmd"

    sleep 1

    if is_cgroup_v2; then
        cgroup_memory_usage=$(cat $CTR_CGROUP/memory.current)
    else 
        cgroup_memory_usage=$(cat $CTR_CGROUP/memory.usage_in_bytes)
    fi

    metrics_memory_usage=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_usage_bytes") | .value.value | tonumber')

    [[ "$cgroup_memory_usage" == "$metrics_memory_usage" ]]
}


@test "verify container_memory_working_set_bytes" {
    metrics_setup

    set_container_pod_cgroup_root "" "$CONTAINER_ID"
    
    cmd='myarray=(); for i in {1..1000}; do myarray+=(\"$(date)\"); done'
    crictl exec  --sync "$CONTAINER_ID" /bin/sh -c "$cmd" 

    # Wait a bit for metrics sync between the cgroup and the crio metrics
    sleep 1

    if is_cgroup_v2; then
        cgroup_memory_inactive_file=$(cat $CTR_CGROUP/memory.stat | grep -w inactive_file | awk '{print $2}') 
        cgroup_memory_usage=$(cat $CTR_CGROUP/memory.current)
    else
        cgroup_memory_inactive_file=$(cat $CTR_CGROUP/memory.stat | grep -w total_inactive_file | awk '{print $2}')
        cgroup_memory_usage=$(cat $CTR_CGROUP/memory.usage_in_bytes)
    fi

    metrics_memory_working_set=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_working_set_bytes") | .value.value | tonumber')

    # Working set should be equal to memory usage - inactive file
    [[ $metrics_memory_working_set == $((cgroup_memory_usage - cgroup_memory_inactive_file)) ]]
}

@test "verify container_memory_rss" {
    metrics_setup

    set_container_pod_cgroup_root "" "$CONTAINER_ID"
    
    cmd='myarray=(); for i in {1..1000}; do myarray+=(\"$(date)\"); done'
    crictl exec  --sync "$CONTAINER_ID" /bin/sh -c "$cmd" 

    if is_cgroup_v2; then
        # for cgroupv2, rss is memory.stat:anon
        cgroup_memory_rss=$(cat $CTR_CGROUP/memory.stat | grep -w anon | awk '{print $2}')
    else
        cgroup_memory_rss=$(cat $CTR_CGROUP/memory.stat | grep -w total_rss | awk '{print $2}')
    fi

    metrics_memory_rss=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_rss") | .value.value | tonumber')

    [[ $metrics_memory_rss == $cgroup_memory_rss ]]
}

@test "verify container_memory_cache" {

    metrics_setup

    set_container_pod_cgroup_root "" "$CONTAINER_ID"
    
    cmd='myarray=(); touch /dev/tmpfile; for i in {1..100}; do myarray+=(\"$(date)\"); date >> /dev/tmpfile; done'
    crictl exec  --sync "$CONTAINER_ID" /bin/sh -c "$cmd"

    if is_cgroup_v2; then
        cgroup_memory_cache=$(cat $CTR_CGROUP/memory.stat | grep -w file | awk '{print $2}')
    else
        cgroup_memory_cache=$(cat $CTR_CGROUP/memory.stat | grep -w cache | awk '{print $2}')
    fi
    metrics_memory_cache=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_cache") | .value.value | tonumber') 

    [[ $metrics_memory_cache == $cgroup_memory_cache ]]
}

@test "verify container_memory_kernel_usage" {
    # Ignored for cgroupv2
    if is_cgroup_v2; then
        skip
    fi

    metrics_setup
    set_container_pod_cgroup_root "" "$CONTAINER_ID"

    # The idea here is to create multiple background processes that should end up consuming kernel memory
    cmd='for i in $(seq 1 1000); do sleep 2 & done; wait;'
    crictl exec --sync "$CONTAINER_ID" /bin/sh -c "$cmd"

    cgroup_memory_kernel_usage=$(cat $CTR_CGROUP/memory.kmem.usage_in_bytes)
    metrics_memory_kernel_usage=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_kernel_usage_bytes") | .value.value | tonumber')

    [[ $metrics_memory_kernel_usage == $cgroup_memory_kernel_usage ]]
}

@test "verify container_memory_max_usage_bytes" {
    # Skipped for cgroupv2
    if is_cgroup_v2; then
        skip
    fi

    metrics_setup
    set_container_pod_cgroup_root "" "$CONTAINER_ID" 

    cmd='for i in {1..10}; do dd if=/dev/zero of=/dev/null bs=10M count=1; done'
    crictl exec --sync "$CONTAINER_ID" /bin/sh -c "$cmd"

    metrics=$(crictl metricsp) && \
    cgroup_memory_max_usage_bytes=$(cat $CTR_CGROUP/memory.max_usage_in_bytes) && \
    metrics_memory_max_usage_bytes=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_max_usage_bytes") | .value.value | tonumber')

    [[ $metrics_memory_max_usage_bytes == $cgroup_memory_max_usage_bytes ]]
}

@test "verify container_memory_failcnt" {
    # Skipped for cgroupv2
    if is_cgroup_v2; then
        skip
    fi

    metrics_setup
    set_container_pod_cgroup_root "" "$CONTAINER_ID" 

    cmd='for i in {1..10}; do dd if=/dev/zero of=/dev/null bs=10M count=10; done'

    crictl exec --sync "$CONTAINER_ID" /bin/sh -c "$cmd"

    metrics=$(crictl metricsp) && \
    cgroup_memory_failcnt=$(cat $CTR_CGROUP/memory.failcnt) && \
    metrics_memory_failcnt=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_failcnt") | .value.value | tonumber')

    [[ $metrics_memory_failcnt == $cgroup_memory_failcnt ]]
}

@test "verify container_memory_mapped_file" {
    skip
    # TODO: find a suitable command to use to increase the mapped file count in the cgroup
}
@test "verify container_memory_swap" {
    skip
    # TODO: find a suitable command to use to increase the swap count in the cgroup
}
