#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	newconfig="$TESTDIR/config.json"
}

function teardown() {
	cleanup_test
}

@test "pids limit" {
	if ! grep -qEw ^pids /proc/cgroups; then
		skip "pids cgroup controller is not available"
	fi
	CONTAINER_PIDS_LIMIT=1234 start_crio

	jq '	  .command'='["/bin/sleep", "600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container_pids_limit.json

	ctr_id=$(crictl run "$TESTDIR"/container_pids_limit.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec --sync "$ctr_id" sh -c 'cat /sys/fs/cgroup/pids/pids.max 2>/dev/null || cat /sys/fs/cgroup/pids.max')
	[[ "$output" == "1234" ]]
}

@test "conmon pod cgroup" {
	CONTAINER_CGROUP_MANAGER="systemd" CONTAINER_DROP_INFRA_CTR=false CONTAINER_CONMON_CGROUP="pod" start_crio

	jq '	  .linux.cgroup_parent = "Burstablecriotest123.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_config_slice.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_config_slice.json)

	output=$(systemctl status "crio-conmon-$pod_id.scope")
	[[ "$output" == *"Burstablecriotest123.slice"* ]]
}

@test "conmon custom cgroup" {
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

	CONTAINER_CGROUP_MANAGER="systemd" CONTAINER_DROP_INFRA_CTR=false CONTAINER_MANAGE_NS_LIFECYCLE=false CONTAINER_CONMON_CGROUP="customcrioconmon.slice" start_crio

	jq '	  .linux.cgroup_parent = "Burstablecriotest123.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_config_slice.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_config_slice.json)

	output=$(systemctl status "crio-conmon-$pod_id.scope")
	[[ "$output" == *"customcrioconmon.slice"* ]]
}

@test "ctr with swap should be configured" {
	if ! grep -v Filename < /proc/swaps; then
		skip "swap not enabled"
	fi
	start_crio
	# memsw should be greater than or equal to memory limit
	# 210763776 = 1024*1024*200
	jq '	  .linux.resources.memory_swap_limit_in_bytes = 210763776
	 	|     .linux.resources.memory_limit_in_bytes = 209715200' \
		"$TESTDATA"/container_sleep.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)
	set_swap_fields_given_cgroup_version

	if test -r "$CGROUP_MEM_SWAP_FILE"; then
		output=$(crictl exec --sync "$ctr_id" sh -c "cat $CGROUP_MEM_SWAP_FILE")
		[[ "$output" == "210763776" ]]
	fi
}

@test "ctr with swap should fail when swap is lower" {
	if ! grep -v Filename < /proc/swaps; then
		skip "swap not enabled"
	fi
	start_crio
	# memsw should be greater than or equal to memory limit
	# 210763776 = 1024*1024*200
	jq '	  .linux.resources.memory_swap_limit_in_bytes = 209715200
	    |     .linux.resources.memory_limit_in_bytes = 210763776' \
		"$TESTDATA"/container_sleep.json > "$newconfig"

	! crictl run "$newconfig" "$TESTDATA"/sandbox_config.json
}

@test "ctr swap only configured if enabled" {
	set_swap_fields_given_cgroup_version
	if test -r "$CGROUP_MEM_SWAP_FILE"; then
		skip "swap cgroup enabled"
	fi
	start_crio
	# memsw should be greater than or equal to memory limit
	# 210763776 = 1024*1024*200
	jq '	  .linux.resources.memory_swap_limit_in_bytes = 210763776
	 	|     .linux.resources.memory_limit_in_bytes = 209715200' \
		"$TESTDATA"/container_sleep.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)
	# verify CRI-O did not specify memory swap value
	jq -e .linux.resources.memory.swap "$(runtime list | grep "$ctr_id" | awk '{ print $4 }')/config.json"
}

@test "ctr with swap should succeed when swap is unlimited" {
	if ! grep -v Filename < /proc/swaps; then
		skip "swap not enabled"
	fi
	start_crio
	# memsw should be greater than or equal to memory limit
	# 210763776 = 1024*1024*200
	jq '	  .linux.resources.memory_swap_limit_in_bytes = -1
	    |     .linux.resources.memory_limit_in_bytes = 210763776' \
		"$TESTDATA"/container_sleep.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)

	if test -r "$CGROUP_MEM_SWAP_FILE"; then
		output=$(crictl exec --sync "$ctr_id" sh -c "cat $CGROUP_MEM_SWAP_FILE")
		[[ $output -gt 210763776 ]]
	fi
}

@test "cgroupv2 unified support" {
	if ! is_cgroup_v2; then
		skip "node must be configured with cgroupv2 for this test"
	fi
	start_crio

	jq '	  .linux.resources.unified = {"memory.min": "209715200", "memory.high": "210763776"}' \
		"$TESTDATA"/container_sleep.json > "$newconfig"
	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/memory.min")
	[[ "$output" == *"209715200"* ]]
	output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/memory.high")
	[[ "$output" == *"210763776"* ]]
}
