#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	newconfig="$TESTDIR/config.json"
	sboxconfig="$TESTDIR/sandbox.json"
	if [[ $RUNTIME_TYPE == vm ]]; then
		skip "not applicable to vm runtime type"
	fi
}

function teardown() {
	cleanup_test
}

function configure_monitor_cgroup_for_conmonrs() {
	local MONITOR_CGROUP="$1"
	local NAME=conmonrs
	cat << EOF > "$CRIO_CONFIG_DIR/01-$NAME.conf"
[crio.runtime]
default_runtime = "$NAME"
[crio.runtime.runtimes.$NAME]
runtime_path = "$RUNTIME_BINARY_PATH"
runtime_root = "$RUNTIME_ROOT"
runtime_type = "$RUNTIME_TYPE"
monitor_cgroup = "$MONITOR_CGROUP"
EOF
	unset CONTAINER_DEFAULT_RUNTIME
	unset CONTAINER_RUNTIMES
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

	CONTAINER_CGROUP_MANAGER="systemd" CONTAINER_DROP_INFRA_CTR=false CONTAINER_CONMON_CGROUP="customcrioconmon.slice" start_crio

	jq '	  .linux.cgroup_parent = "Burstablecriotest123.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_config_slice.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_config_slice.json)

	output=$(systemctl status "crio-conmon-$pod_id.scope")
	[[ "$output" == *"customcrioconmon.slice"* ]]
}

@test "conmon custom cgroup with no infra container" {
	parent="Burstablecriotest123"
	if [ "$CONTAINER_CGROUP_MANAGER" == "systemd" ]; then
		parent="$parent".slice
	fi
	cgroup_base="/sys/fs/cgroup"
	if ! is_cgroup_v2; then
		cgroup_base="$cgroup_base"/memory
	fi

	CONTAINER_DROP_INFRA_CTR=true start_crio

	jq --arg cg "$parent" '	  .linux.cgroup_parent = $cg' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_config_slice.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_config_slice.json)
	ls "$cgroup_base"/"$parent"/crio-"$pod_id"*

	crictl rmp -fa
	run ! ls "$cgroup_base"/"$parent"/crio-"$pod_id"*
}

@test "conmonrs custom cgroup with no infra container" {
	if [[ $RUNTIME_TYPE != pod ]]; then
		skip "not supported for conmon"
	fi

	setup_crio
	configure_monitor_cgroup_for_conmonrs "customcrioconmon.slice"
	CONTAINER_CGROUP_MANAGER="systemd" CONTAINER_DROP_INFRA_CTR=true start_crio_no_setup

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

	run ! crictl run "$newconfig" "$TESTDATA"/sandbox_config.json
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

@test "cpu-quota.crio.io can disable quota" {
	if is_cgroup_v2; then
		skip "node must be configured with cgroupv1 for this test"
	fi
	create_workload_with_allowed_annotation cpu-quota.crio.io

	start_crio

	jq '   .annotations["cpu-quota.crio.io"] = "disable"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq '   .annotations["cpu-quota.crio.io"] = "disable" |
	       .linux.resources.cpu_shares = 1024 ' \
		"$TESTDATA"/container_sleep.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$sboxconfig")
	set_container_pod_cgroup_root "cpu" "$ctr_id"
	# TODO: add support for cgroupv2 when cpu load balancing is supported there.
	cgroup_file="cpu.cfs_quota_us"
	[[ $(cat "$CTR_CGROUP"/"$cgroup_file") == "-1" ]]
	[[ $(cat "$POD_CGROUP"/"$cgroup_file") == "-1" ]]
	if [[ "$CONTAINER_DEFAULT_RUNTIME" == "crun" ]]; then
		[[ $(cat "$CTR_CGROUP"/container/"$cgroup_file") == "-1" ]]
	fi
}

@test "systemd cgroup manager uses system dbus when running as root with rootless env" {
	if [[ $(id -u) -ne 0 ]]; then
		skip "test requires running as root"
	fi

	# Set _CRIO_ROOTLESS=1 to simulate containerized environment (like in nested containers)
	# where rootless adjustments are needed but system dbus should still be used.
	_CRIO_ROOTLESS=1 CONTAINER_CGROUP_MANAGER="systemd" CONTAINER_DROP_INFRA_CTR=false start_crio

	jq '	  .linux.cgroup_parent = "system.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_systemd.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_systemd.json)

	output=$(systemctl status "crio-conmon-$pod_id.scope" 2>&1) || true
	[[ "$output" == *"crio-conmon-$pod_id.scope"* ]]

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_systemd.json)
	crictl start "$ctr_id"

	output=$(crictl inspect "$ctr_id" | jq -r '.status.state')
	[[ "$output" == "CONTAINER_RUNNING" ]]
}
