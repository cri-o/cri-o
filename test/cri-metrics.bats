#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function metrics_setup() {
	# start sandbox
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	# Make sure we get a non-empty metrics response
	crictl metricsp | grep "podSandboxId"

	jq --arg TESTDATA "$TESTDATA" '.mounts = [{
            host_path: $TESTDATA,
            container_path: "/testdata",
          }]' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container_metrics.json"
	CONTAINER_ID=$(crictl create "$POD_ID" "$TESTDIR/container_metrics.json" "$TESTDATA/sandbox_config.json")
	crictl start "$CONTAINER_ID"

	# assert pod metrics are present
	crictl metricsp | grep "container_network_receive_bytes_total"
	# assert container metrics are present
	crictl metricsp | grep "container_memory_usage_bytes"
}

@test "container memory metrics" {
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT=$(free_port) setup_crio
	cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = [
    "network",
    "cpu",
    "hugetlb",
    "memory",
    "oom",
]
EOF
	start_crio_no_setup
	check_images

	metrics_setup
	set_container_pod_cgroup_root "" "$CONTAINER_ID"

	cmd='for i in {1..10}; do dd if=/dev/zero of=/dev/null bs=10M count=1; done'
	crictl exec --sync "$CONTAINER_ID" /bin/sh -c "$cmd"
	# wait a bit for metrics sync - tests are more flaky without this
	sleep 1

	# assert container_memory_usage_bytes == cgroup memory.usage_in_bytes(cgroup v1) or memory.current(cgroup v2)
	if is_cgroup_v2; then
		cgroup_memory_usage=$(cat "$CTR_CGROUP"/memory.current)
	else
		cgroup_memory_usage=$(cat "$CTR_CGROUP"/memory.usage_in_bytes)
	fi
	metrics_memory_usage=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_usage_bytes") | .value.value | tonumber')
	[[ "$cgroup_memory_usage" == "$metrics_memory_usage" ]]

	# assert container_memory_working_set_bytes ==
	#    cgroup memory.usage_in_bytes - cgroup memory.stat:total_inactive_file(cgroup v1) or memory.current - memory.stat:inactive_file(cgroup v2)
	if is_cgroup_v2; then
		cgroup_memory_inactive_file=$(grep -w inactive_file < "$CTR_CGROUP"/memory.stat | awk '{print $2}')
		cgroup_memory_usage=$(cat "$CTR_CGROUP"/memory.current)
	else
		cgroup_memory_inactive_file=$(grep -w total_inactive_file < "$CTR_CGROUP"/memory.stat | awk '{print $2}')
		cgroup_memory_usage=$(cat "$CTR_CGROUP"/memory.usage_in_bytes)
	fi

	metrics_memory_working_set=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_working_set_bytes") | .value.value | tonumber')
	[[ $metrics_memory_working_set == $((cgroup_memory_usage - cgroup_memory_inactive_file)) ]]

	# assert container_memory_rss == cgroup memory.stat:total_rss(cgroup v1) or memory.stat:anon(cgroup v2)
	if is_cgroup_v2; then
		# for cgroupv2, rss is memory.stat:anon
		cgroup_memory_rss=$(grep -w anon < "$CTR_CGROUP"/memory.stat | awk '{print $2}')
	else
		cgroup_memory_rss=$(grep -w total_rss "$CTR_CGROUP"/memory.stat | awk '{print $2}')
	fi
	metrics_memory_rss=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_rss") | .value.value | tonumber')
	[[ $metrics_memory_rss == "$cgroup_memory_rss" ]]

	cmd="myarray=(); touch /dev/tmpfile; for i in {1..100}; do myarray+=(\"$(date)\"); date >> /dev/tmpfile; done"
	crictl exec --sync "$CONTAINER_ID" /bin/sh -c "$cmd"

	# assert container_memory_cache == cgroup memory.stat:cache(cgroup v1) or memory.stat:file(cgroup v2)
	if is_cgroup_v2; then
		cgroup_memory_cache=$(grep -w file < "$CTR_CGROUP"/memory.stat | awk '{print $2}')
	else
		cgroup_memory_cache=$(grep -w cache < "$CTR_CGROUP"/memory.stat | awk '{print $2}')
	fi
	metrics_memory_cache=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_cache") | .value.value | tonumber')
	[[ $metrics_memory_cache == "$cgroup_memory_cache" ]]

	# assert container_memory_swap == cgroup memory.swap.current(cgroup v2).
	# or for cgroup v1, container_memory_swap == cgroup memory.memsw.usage_in_bytes - cgroup memory.usage_in_bytes
	# (why?) because cgroup v1 reports swap as usage+swap in memory.memsw.usage_in_bytes, while crio reports only the swap value

	# TODO: find a suitable command/script to use to increase the swap value in the cgroup

	# assert  container_memory_mapped_file == cgroup memory.stat:file_mapped (cgroup v2)
	# or cgroup memory.stat:total_mapped_file (cgroup v1 hierarchy)
	# or cgroup memory.stat:mapped_file (cgroup v1)

	# TODO: find a suitable command/script to use to increase the mapped file count in the cgroup
}

@test "container memory cgroupv1-specific metrics" {
	if is_cgroup_v2; then
		skip "skip test for cgroup v2"
	fi

	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT=$(free_port) setup_crio
	cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = [
    "network",
    "cpu",
    "hugetlb",
    "memory",
    "oom",
]
EOF
	start_crio_no_setup
	check_images

	metrics_setup
	set_container_pod_cgroup_root "" "$CONTAINER_ID"

	# The idea here is to create multiple background processes that should end up consuming kernel memory
	cmd="for i in $(seq 1 1000); do sleep 2 & done; wait;"
	crictl exec --sync "$CONTAINER_ID" /bin/sh -c "$cmd"
	# wait a bit for metrics sync - tests are more flaky without this
	sleep 1

	# assert container_memory_kernel_usage_bytes == cgroup memory.kmem.usage_in_bytes
	cgroup_memory_kernel_usage=$(cat "$CTR_CGROUP"/memory.kmem.usage_in_bytes) &&
		metrics_memory_kernel_usage=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_kernel_usage") | .value.value | tonumber')
	[[ $metrics_memory_kernel_usage == "$cgroup_memory_kernel_usage" ]]

	cmd='for i in {1..10}; do dd if=/dev/zero of=/dev/null bs=10M count=1; done'
	crictl exec --sync "$CONTAINER_ID" /bin/sh -c "$cmd"

	# assert container_memory_max_usage_bytes == cgroup memory.max_usage_in_bytes
	metrics=$(crictl metricsp) &&
		cgroup_memory_max_usage_bytes=$(cat "$CTR_CGROUP"/memory.max_usage_in_bytes)
	metrics_memory_max_usage_bytes=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_max_usage_bytes") | .value.value | tonumber')
	[[ $metrics_memory_max_usage_bytes == "$cgroup_memory_max_usage_bytes" ]]

	# assert container_memory_failcnt == cgroup memory.failcnt
	metrics=$(crictl metricsp) &&
		cgroup_memory_failcnt=$(cat "$CTR_CGROUP"/memory.failcnt) &&
		metrics_memory_failcnt=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_memory_failcnt") | .value.value | tonumber')
	[[ $metrics_memory_failcnt == "$cgroup_memory_failcnt" ]]
}

@test "container hugetlb metrics" {
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT=$(free_port) setup_crio
	cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = [
    "network",
    "cpu",
    "hugetlb",
    "memory",
    "oom",
]
EOF
	start_crio_no_setup
	check_images

	metrics_setup
	set_container_pod_cgroup_root "" "$CONTAINER_ID"

	# allocate static huge pages
	old_pages=$(cat /proc/sys/vm/nr_hugepages)
	if [[ $old_pages == "0" ]]; then
		echo 1 | tee /proc/sys/vm/nr_hugepages

		bats::on_failure() {
			echo 0 | tee /proc/sys/vm/nr_hugepages
		}
	fi

	# make use of the huge page in the container
	crictl exec --sync "$CONTAINER_ID" /usr/bin/cp /testdata/usehugetlb.c /
	crictl exec --sync "$CONTAINER_ID" /usr/bin/gcc /usehugetlb.c -o /usr/bin/usehugetlb
	crictl exec --sync "$CONTAINER_ID" /usr/bin/usehugetlb &

	# wait until the huge page is being consumed
	until [[ $(cat "$CTR_CGROUP"/hugetlb.2MB.rsvd.current) == "2097152" ]]; do
		sleep 1
	done

	metrics=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[]')

	# assert container_hugetlb_usage_bytes{pagesize="2MB"} == 2MB
	metrics_hugetlb_usage_2mb=$(echo "$metrics" | jq 'select(.name == "container_hugetlb_usage_bytes" and any(.labelValues[]; . == "2MB")) | .value.value | tonumber')
	[[ $metrics_hugetlb_usage_2mb == "2097152" ]]

	# assert container_hugetlb_max_usage_bytes{pagesize="2MB"} == 0(cgroup v2) or 2MB(cgroup v1)
	# cgroup v2 does not support hugetlb max usage stats
	metrics_hugetlb_max_usage_2mb=$(echo "$metrics" | jq 'select(.name == "container_hugetlb_max_usage_bytes" and any(.labelValues[]; . == "2MB")) | .value.value | tonumber')
	if is_cgroup_v2; then
		cgroup_hugetlb_max_usage_2mb=0
	else
		cgroup_hugetlb_max_usage_2mb=2097152
	fi
	[[ $metrics_hugetlb_max_usage_2mb == "$cgroup_hugetlb_max_usage_2mb" ]]

	# assert container_hugetlb_usage_bytes{pagesize="1GB"} == 0
	metrics_hugetlb_usage_1gb=$(echo "$metrics" | jq 'select(.name == "container_hugetlb_usage_bytes" and any(.labelValues[]; . == "1GB")) | .value.value | tonumber')
	[[ $metrics_hugetlb_usage_1gb == "0" ]]

	# assert container_hugetlb_max_usage_bytes{pagesize="1GB"} == 0
	metrics_hugetlb_max_usage_1gb=$(echo "$metrics" | jq 'select(.name == "container_hugetlb_max_usage_bytes" and any(.labelValues[]; . == "1GB")) | .value.value | tonumber')
	[[ $metrics_hugetlb_max_usage_1gb == "0" ]]

	# cleanup
	if [[ $old_pages == "0" ]]; then
		echo 0 | tee /proc/sys/vm/nr_hugepages
	fi
}
