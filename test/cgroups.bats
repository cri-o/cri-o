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

	python -c 'import json,sys;obj=json.load(sys.stdin); obj["command"] = ["/bin/sleep", "600"]; json.dump(obj, sys.stdout)' \
		< "$TESTDATA"/container_config.json > "$TESTDIR"/container_pids_limit.json

	ctr_id=$(crictl run "$TESTDIR"/container_pids_limit.json "$TESTDATA"/sandbox_config.json)

	run crictl exec --sync "$ctr_id" cat /sys/fs/cgroup/pids/pids.max
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "1234" ]]
}

@test "conmon custom cgroup" {
	CONTAINER_CGROUP_MANAGER="systemd" CONTAINER_CONMON_CGROUP="customcrioconmon.slice" start_crio

	python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["cgroup_parent"] = "Burstablecriotest123.slice"; json.dump(obj, sys.stdout)' \
		< "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_config_slice.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_config_slice.json)

	run systemctl status "crio-conmon-$pod_id.scope"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *"customcrioconmon.slice"* ]]
}
