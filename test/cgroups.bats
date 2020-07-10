#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "pids limit" {
	if ! grep pids /proc/self/cgroup; then
		skip "pids cgroup controller is not mounted"
	fi
	CONTAINER_PIDS_LIMIT=1234 start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	pids_limit_config=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin); obj["command"] = ["/bin/sleep", "600"]; json.dump(obj, sys.stdout)')
	echo "$pids_limit_config" > "$TESTDIR"/container_pids_limit.json
	run crictl create "$pod_id" "$TESTDIR"/container_pids_limit.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" cat /sys/fs/cgroup/pids/pids.max
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "1234" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "conmon custom cgroup" {
	# TODO FIXME we should still probably have a parent cgroup, right?
	CONTAINER_CGROUP_MANAGER="systemd" CONTAINER_MANAGE_NS_LIFECYCLE=false CONTAINER_DROP_INFRA=false CONTAINER_CONMON_CGROUP="customcrioconmon.slice" start_crio
	cgroup_parent_config=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["cgroup_parent"] = "Burstablecriotest123.slice"; json.dump(obj, sys.stdout)')
	echo "$cgroup_parent_config" > "$TESTDIR"/sandbox_config_slice.json
	run crictl runp "$TESTDIR"/sandbox_config_slice.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	scope_name="crio-conmon-$pod_id.scope"
	run systemctl status $scope_name
	[[ "$output" =~ "customcrioconmon.slice" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}
