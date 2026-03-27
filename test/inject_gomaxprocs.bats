#!/usr/bin/env bats
# shellcheck disable=SC2154

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "inject_gomaxprocs for burstable pod auto-calculates from CPU shares" {
	CONTAINER_INJECT_GOMAXPROCS=4 start_crio

	# Burstable cgroup parent with cpu_shares=8192 (8 CPU request)
	jq '  .linux.cgroup_parent = "burstable-pod_test.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_burstable.json

	jq '  .linux.resources.cpu_shares = 8192
	    | .linux.resources.cpu_quota = 0
	    | .linux.resources.cpu_period = 0' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/ctr_config.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_burstable.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/ctr_config.json "$TESTDIR"/sandbox_burstable.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" == *"GOMAXPROCS=8"* ]]
}

@test "inject_gomaxprocs uses floor when calculated value is lower" {
	CONTAINER_INJECT_GOMAXPROCS=4 start_crio

	# Burstable cgroup parent with cpu_shares=512 (500m request) -> ceil(512/1024)=1, floor=4
	jq '  .linux.cgroup_parent = "burstable-pod_test.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_burstable.json

	jq '  .linux.resources.cpu_shares = 512
	    | .linux.resources.cpu_quota = 0
	    | .linux.resources.cpu_period = 0' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/ctr_config.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_burstable.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/ctr_config.json "$TESTDIR"/sandbox_burstable.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" == *"GOMAXPROCS=4"* ]]
}

@test "inject_gomaxprocs uses fallback for besteffort pod" {
	CONTAINER_INJECT_GOMAXPROCS=4 start_crio

	# BestEffort cgroup parent, no meaningful shares
	jq '  .linux.cgroup_parent = "besteffort-pod_test.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_be.json

	jq '  .linux.resources.cpu_shares = 2
	    | .linux.resources.cpu_quota = 0
	    | .linux.resources.cpu_period = 0' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/ctr_config.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_be.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/ctr_config.json "$TESTDIR"/sandbox_be.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" == *"GOMAXPROCS=4"* ]]
}

@test "inject_gomaxprocs skips guaranteed pod" {
	CONTAINER_INJECT_GOMAXPROCS=4 start_crio

	# Guaranteed pod (no burstable/besteffort in cgroup parent)
	jq '  .linux.cgroup_parent = "guaranteed-pod_test.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_guar.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_guar.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_guar.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" != *"GOMAXPROCS="* ]]
}

@test "inject_gomaxprocs preserves existing GOMAXPROCS from pod spec" {
	CONTAINER_INJECT_GOMAXPROCS=4 start_crio

	jq '  .linux.cgroup_parent = "burstable-pod_test.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_burstable.json

	# Container with GOMAXPROCS already set
	jq '  .envs += [{"key": "GOMAXPROCS", "value": "16"}]
	    | .linux.resources.cpu_shares = 2048
	    | .linux.resources.cpu_quota = 0
	    | .linux.resources.cpu_period = 0' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/ctr_config.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_burstable.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/ctr_config.json "$TESTDIR"/sandbox_burstable.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" == *"GOMAXPROCS=16"* ]]
}

@test "inject_gomaxprocs disabled when set to 0" {
	CONTAINER_INJECT_GOMAXPROCS=0 start_crio

	jq '  .linux.cgroup_parent = "burstable-pod_test.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_burstable.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_burstable.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_burstable.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" != *"GOMAXPROCS="* ]]
}

@test "inject_gomaxprocs skips burstable pod with CPU limit set" {
	CONTAINER_INJECT_GOMAXPROCS=4 start_crio

	# Burstable because memory request != limit, but has a CPU limit.
	# Go 1.25+ auto-detects GOMAXPROCS from quota, so we should not inject.
	jq '  .linux.cgroup_parent = "burstable-pod_test.slice"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_burstable.json

	jq '  .linux.resources.cpu_shares = 2048
	    | .linux.resources.cpu_quota = 200000
	    | .linux.resources.cpu_period = 100000' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/ctr_config.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_burstable.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/ctr_config.json "$TESTDIR"/sandbox_burstable.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" != *"GOMAXPROCS="* ]]
}

@test "inject_gomaxprocs skips pod with skip annotation" {
	CONTAINER_INJECT_GOMAXPROCS=4 start_crio

	jq '  .linux.cgroup_parent = "burstable-pod_test.slice"
	    | .annotations += {"skip-gomaxprocs.crio.io": "true"}' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_skip.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_skip.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_skip.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" != *"GOMAXPROCS="* ]]
}
