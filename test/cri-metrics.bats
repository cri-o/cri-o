#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

# wait_for_metric polls crictl metricsp until the given metric name appears
function wait_for_metric() {
	local metric_name="$1"
	local timeout=60
	local i

	for i in $(seq 1 $timeout); do
		if crictl metricsp 2> /dev/null | grep -q "$metric_name"; then
			return 0
		fi
		sleep 1
	done

	echo "Timed out waiting for metric: $metric_name" >&2
	crictl metricsp || true
	return 1
}

function metrics_setup() {
	# start sandbox
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	# Make sure we get a non-empty metrics response
	crictl metricsp | grep "podSandboxId"

	# set memory.min for container_spec_memory_reservation_limit_bytes
	jq --arg TESTDATA "$TESTDATA" '.mounts = [{
            host_path: $TESTDATA,
            container_path: "/testdata",
          }] | .linux.resources.unified["memory.min"] = "134217728"' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container_metrics.json"
	CONTAINER_ID=$(crictl create "$POD_ID" "$TESTDIR/container_metrics.json" "$TESTDATA/sandbox_config.json")
	crictl start "$CONTAINER_ID"

	# assert pod metrics are present
	crictl metricsp | grep "container_network_receive_bytes_total"
	# assert container metrics are present
	crictl metricsp | grep "container_memory_usage_bytes"
	# assert metadata is there
	crictl metricsp | grep "$CONTAINER_ID"
}

@test "empty included_pod_metrics returns always-on metrics and doesn't return any not-included metrics" {
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT=$(free_port) setup_crio
	cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = []
EOF

	start_crio_no_setup

	alwaysOn=(
		container_last_seen
	)

	for desc in $(crictl metricdescs | jq -r ".descriptors.[].name"); do
		# check if the desc is in alwaysOn
		[[ " ${alwaysOn[*]} " == *" $desc "* ]]
	done
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
	"disk",
    "hugetlb",
    "memory",
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

@test "container disk metrics" {
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT=$(free_port) setup_crio
	cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = [
    "network",
	"disk",
    "memory",
]
EOF
	start_crio_no_setup
	check_images

	metrics_setup
	set_container_pod_cgroup_root "" "$CONTAINER_ID"

	wait_for_metric "container_fs_usage_bytes"

	metrics=$(crictl metricsp)

	# assert container_fs_usage_bytes is present
	fs_usage=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_usage_bytes") | .value.value | tonumber')
	[[ "$fs_usage" -gt 0 ]]

	# assert container_fs_limit_bytes is present
	fs_limit=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_limit_bytes") | .value.value | tonumber')
	[[ "$fs_limit" -gt 0 ]]

	# assert container_fs_inodes_free is present
	fs_inodes_free=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_inodes_free") | .value.value | tonumber')
	[[ "$fs_inodes_free" -gt 0 ]]

	# assert container_fs_inodes_total is present
	fs_inodes_total=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_inodes_total") | .value.value | tonumber')
	[[ "$fs_inodes_total" -gt 0 ]]

	# Validate that inodes_total >= inodes_free (basic sanity check)
	[[ "$fs_inodes_total" -ge "$fs_inodes_free" ]]

	# Test inode metrics by creating many small files
	# Create 50 small files to consume inodes
	crictl exec --sync "$CONTAINER_ID" mkdir -p /var/lib/mydisktest
	crictl exec --sync "$CONTAINER_ID" /bin/sh -c "for i in \$(seq 1 50); do touch /var/lib/mydisktest/inode_test_file_\$i; done"
	crictl exec --sync "$CONTAINER_ID" sync

	# Poll for inode metrics to be updated
	local timeout=60 # Set a reasonable timeout in seconds
	local new_fs_inodes_free=0
	local found_inode_decrease=false
	for ((i = 0; i < timeout; i++)); do
		new_fs_inodes_free=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_inodes_free") | .value.value | tonumber')
		if [[ "$new_fs_inodes_free" -lt "$fs_inodes_free" ]]; then
			found_inode_decrease=true
			break
		fi
		sleep 1
	done

	if ! $found_inode_decrease; then
		echo "Free inode count did not decrease within $timeout seconds after creating files"
		exit 1
	fi

	# Verify inode metrics consistency after file creation
	new_metrics=$(crictl metricsp)
	new_fs_inodes_total=$(echo "$new_metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_inodes_total") | .value.value | tonumber')
	# Total inodes should remain the same
	[[ "$new_fs_inodes_total" -eq "$fs_inodes_total" ]]
	# Free inodes should be less than before
	[[ "$new_fs_inodes_free" -lt "$fs_inodes_free" ]]
	# Ensure total >= free still holds
	[[ "$new_fs_inodes_total" -ge "$new_fs_inodes_free" ]]

	# Generate disk usage and validate increase
	crictl exec --sync "$CONTAINER_ID" mkdir -p /var/lib/mydisktest
	crictl exec --sync "$CONTAINER_ID" dd if=/dev/zero of=/var/lib/mydisktest/bloatfile bs=1024 count=4
	crictl exec --sync "$CONTAINER_ID" sync
	# Polling loop for metrics to be updated
	local timeout=60 # Set a reasonable timeout in seconds
	local new_fs_usage=0
	local found_increase=false
	for ((i = 0; i < timeout; i++)); do
		new_fs_usage=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_usage_bytes") | .value.value | tonumber')
		if [[ "$new_fs_usage" -gt "$fs_usage" ]]; then
			found_increase=true
			break
		fi
		sleep 1
	done

	if ! $found_increase; then
		echo "Disk usage did not increase within $timeout seconds"
		exit 1
	fi

	stop_crio
}

@test "container disk io metrics" {
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT=$(free_port) setup_crio
	cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = [
    "network",
	"diskIO",
    "memory",
]
EOF

	start_crio_no_setup
	check_images

	metrics_setup
	set_container_pod_cgroup_root "" "$CONTAINER_ID"

	# Test inode metrics by creating many small files
	# Create 50 small files to consume inodes
	crictl exec --sync "$CONTAINER_ID" mkdir -p /var/lib/mydisktest
	crictl exec --sync "$CONTAINER_ID" /bin/sh -c "for i in \$(seq 1 10); do echo hi >> /var/lib/mydisktest/inode_test_file_\$i; done"
	crictl exec --sync "$CONTAINER_ID" sync

	metrics=$(crictl metricsp)

	fs_reads_bytes=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_reads_bytes_total") | .value.value | tonumber' | sort -n | tail -n 1)
	[[ "$fs_reads_bytes" != "" ]] # reads are difficult if we've just written, the file is cached in memory

	fs_writes_bytes=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_writes_bytes_total") | .value.value | tonumber' | sort -n | tail -n 1)
	[[ "$fs_writes_bytes" -gt 0 ]]

	fs_reads=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_reads_total") | .value.value | tonumber' | sort -n | tail -n 1)
	[[ "$fs_reads" != "" ]] # reads are difficult if we've just written, the file is cached in memory

	fs_writes=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_fs_writes_total") | .value.value | tonumber' | sort -n | tail -n 1)
	[[ "$fs_writes" -gt 0 ]]

	blkio=$(echo "$metrics" | jq '.podMetrics[0].containerMetrics[0].metrics[] | select(.name == "container_blkio_device_usage_total") | .value.value | tonumber' | sort -n | tail -n 1)
	[[ "$blkio" != "" ]]
}

@test "container process metrics" {
	SOFT_ULIMIT=1000
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT=$(free_port) CONTAINER_DEFAULT_ULIMITS="nofile=$SOFT_ULIMIT:1025" setup_crio
	cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = [
    "network",
    "memory",
    "process",
]
EOF
	# TODO: having the crun subcgroup breaks this collection
	# remove this when that is fixed
	unset CONTAINER_RUNTIMES
	sed -i '/privileged_without_host_devices = false/a default_annotations = { "run.oci.systemd.subgroup" = "" }' "$CRIO_CUSTOM_CONFIG"

	start_crio_no_setup
	check_images

	metrics_setup
	set_container_pod_cgroup_root "" "$CONTAINER_ID"

	metrics=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[]')

	metrics_container_processes=$(echo "$metrics" | jq 'select(.name == "container_processes") | .value.value | tonumber')
	[[ $metrics_container_processes == "1" ]]

	metrics_container_threads=$(echo "$metrics" | jq 'select(.name == "container_threads") | .value.value | tonumber')
	[[ $metrics_container_threads == "1" ]]

	metrics_file_descriptors=$(echo "$metrics" | jq 'select(.name == "container_file_descriptors") | .value.value | tonumber')
	[[ "$metrics_file_descriptors" == "3" ]]

	metrics_sockets=$(echo "$metrics" | jq 'select(.name == "container_sockets") | .value.value | tonumber')
	[[ "$metrics_sockets" == "0" ]]

	if is_cgroup_v2; then
		cgroup_threads_max=$(cat "$CTR_CGROUP"/pids.max)
	else
		cgroup_threads_max=$(cat "$CTR_CGROUP"/pids.limit)
	fi

	metrics_threads_max=$(echo "$metrics" | jq 'select(.name == "container_threads_max") | .value.value | tonumber')
	if [[ "$cgroup_threads_max" == "max" ]]; then
		# limit is infinity if value is zero
		[[ "$metrics_threads_max" == "0" ]]
	else
		[[ "$cgroup_threads_max" == "$metrics_threads_max" ]]
	fi

	metrics_ulimits_soft=$(echo "$metrics" | jq 'select(.name == "container_ulimits_soft") | .value.value | tonumber')

	[[ "$metrics_ulimits_soft" == "$SOFT_ULIMIT" ]]
}

@test "container last seen metrics" {
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT=$(free_port) setup_crio
	cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = [
    "network",
    "memory",
]
EOF
	start_crio_no_setup
	check_images

	metrics_setup
	set_container_pod_cgroup_root "" "$CONTAINER_ID"

	metrics=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[]')

	metrics_last_seen=$(echo "$metrics" | jq 'select(.name == "container_last_seen") | .value.value | tonumber')
	[[ $metrics_last_seen != "0" ]]
}

@test "container spec metrics" {
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT=$(free_port) setup_crio
	cat << EOF > "$CRIO_CONFIG"
[crio.stats]
collection_period = 0
included_pod_metrics = [
    "network",
    "memory",
    "spec",
]
EOF
	start_crio_no_setup
	check_images

	metrics_setup
	set_container_pod_cgroup_root "" "$CONTAINER_ID"

	metrics=$(crictl metricsp | jq '.podMetrics[0].containerMetrics[0].metrics[]')

	metrics_cpu_period=$(echo "$metrics" | jq 'select(.name == "container_spec_cpu_period") | .value.value | tonumber')
	[[ "$metrics_cpu_period" == "10000" ]]
	metrics_cpu_shares=$(echo "$metrics" | jq 'select(.name == "container_spec_cpu_shares") | .value.value | tonumber')
	[[ "$metrics_cpu_shares" == "512" ]]
	metrics_cpu_quota=$(echo "$metrics" | jq 'select(.name == "container_spec_cpu_quota") | .value.value | tonumber')
	[[ "$metrics_cpu_quota" == "20000" ]]
	metrics_memory_limit_bytes=$(echo "$metrics" | jq 'select(.name == "container_spec_memory_limit_bytes") | .value.value | tonumber')
	[[ $metrics_memory_limit_bytes == "268435456" ]]
	metrics_memory_swap_limit_bytes=$(echo "$metrics" | jq 'select(.name == "container_spec_memory_swap_limit_bytes") | .value.value | tonumber')
	[[ "$metrics_memory_swap_limit_bytes" == "268435456" || "$metrics_memory_swap_limit_bytes" == "" ]]
	metrics_start_time=$(echo "$metrics" | jq 'select(.name == "container_start_time_seconds") | .value.value | tonumber')
	[[ "$metrics_start_time" != "0" && "$metrics_start_time" != "" ]]
	if is_cgroup_v2; then
		metrics_memory_reservation_limit_bytes=$(echo "$metrics" | jq 'select(.name == "container_spec_memory_reservation_limit_bytes") | .value.value | tonumber')
		[[ $metrics_memory_reservation_limit_bytes == "134217728" ]]
	fi
}
