#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
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

@test "conmon custom cgroup" {
	CONTAINER_CGROUP_MANAGER="systemd" CONTAINER_DROP_INFRA_CTR=false CONTAINER_MANAGE_NS_LIFECYCLE=false CONTAINER_CONMON_CGROUP="customcrioconmon.slice" start_crio

	jq '	  .linux.cgroup_parent = "Burstablecriotest123.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_config_slice.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_config_slice.json)

	output=$(systemctl status "crio-conmon-$pod_id.scope")
	[[ "$output" == *"customcrioconmon.slice"* ]]
}
