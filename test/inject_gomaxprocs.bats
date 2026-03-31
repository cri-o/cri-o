#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
	sboxconfig="$TESTDIR/sbox.json"
	ctrconfig="$TESTDIR/ctr.json"
}

function teardown() {
	cleanup_test
}

function enable_gomaxprocs() {
	local floor="${1:-4}"
	cat << EOF > "$CRIO_CONFIG_DIR/01-gomaxprocs.conf"
[crio.runtime]
min_injected_gomaxprocs = $floor
EOF
}

function create_burstable_sandbox() {
	local name="${1:-burstable-sandbox}"
	jq --arg name "$name" \
		'.metadata.name = $name
		| .linux.cgroup_parent = "kubepods-burstable-pod123.slice"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"
}

function create_besteffort_sandbox() {
	local name="${1:-besteffort-sandbox}"
	jq --arg name "$name" \
		'.metadata.name = $name
		| .linux.cgroup_parent = "kubepods-besteffort-pod123.slice"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"
}

function create_guaranteed_sandbox() {
	local name="${1:-guaranteed-sandbox}"
	jq --arg name "$name" \
		'.metadata.name = $name
		| .linux.cgroup_parent = "kubepods-pod123.slice"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"
}

function get_gomaxprocs() {
	local ctr_id="$1"
	crictl inspect "$ctr_id" | jq -r \
		'[.info.runtimeSpec.process.env[] | select(startswith("GOMAXPROCS="))] | first // "not-set"'
}

# Verify GOMAXPROCS is injected for burstable pods with cpu shares.
@test "min_injected_gomaxprocs for burstable pod uses floor when calculated < floor" {
	enable_gomaxprocs 4
	start_crio

	create_burstable_sandbox

	# 2048 shares = 2 CPU request. ceil(2048/1024) = 2, which is < floor 4.
	jq '.linux.resources.cpu_shares = 2048
		| .linux.resources.cpu_quota = 0' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "GOMAXPROCS=4" ]]
}

# Verify calculated value is used when it exceeds the floor.
@test "min_injected_gomaxprocs for burstable pod uses calculated when > floor" {
	enable_gomaxprocs 4
	start_crio

	create_burstable_sandbox

	# 8192 shares = 8 CPU request. ceil(8192/1024) = 8, which is > floor 4.
	jq '.linux.resources.cpu_shares = 8192
		| .linux.resources.cpu_quota = 0' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "GOMAXPROCS=8" ]]
}

# Verify GOMAXPROCS is injected for best-effort pods (no shares).
@test "min_injected_gomaxprocs for besteffort pod uses floor" {
	enable_gomaxprocs 4
	start_crio

	create_besteffort_sandbox

	# Best-effort: shares=2 (kernel default). Should use floor.
	jq '.linux.resources.cpu_shares = 2
		| .linux.resources.cpu_quota = 0' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "GOMAXPROCS=4" ]]
}

# Verify GOMAXPROCS is NOT injected for guaranteed pods.
@test "min_injected_gomaxprocs skips guaranteed pods" {
	enable_gomaxprocs 4
	start_crio

	create_guaranteed_sandbox

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "not-set" ]]
}

# Verify GOMAXPROCS is NOT injected when container has CPU quota (limit).
@test "min_injected_gomaxprocs skips containers with CPU limit" {
	enable_gomaxprocs 4
	start_crio

	create_burstable_sandbox

	# cpu_quota > 0 means the container has a CPU limit.
	jq '.linux.resources.cpu_shares = 2048
		| .linux.resources.cpu_quota = 200000
		| .linux.resources.cpu_period = 100000' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "not-set" ]]
}

# Verify user-set GOMAXPROCS is preserved (not overridden).
@test "min_injected_gomaxprocs preserves user-set GOMAXPROCS" {
	enable_gomaxprocs 4
	start_crio

	create_burstable_sandbox

	jq '.linux.resources.cpu_shares = 2048
		| .linux.resources.cpu_quota = 0
		| .envs = [{"key": "GOMAXPROCS", "value": "16"}]' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "GOMAXPROCS=16" ]]
}

# Verify GOMAXPROCS is NOT injected when disabled (min_injected_gomaxprocs = 0).
@test "min_injected_gomaxprocs disabled when set to 0" {
	start_crio

	create_burstable_sandbox

	jq '.linux.resources.cpu_shares = 2048
		| .linux.resources.cpu_quota = 0' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "not-set" ]]
}

# Verify the skip annotation prevents GOMAXPROCS injection.
@test "min_injected_gomaxprocs skips pods with skip annotation" {
	enable_gomaxprocs 4
	start_crio

	jq '.metadata.name = "skip-sandbox"
		| .linux.cgroup_parent = "kubepods-burstable-pod123.slice"
		| .annotations["skip-gomaxprocs.crio.io"] = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq '.linux.resources.cpu_shares = 2048
		| .linux.resources.cpu_quota = 0' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "not-set" ]]
}

# Verify workload-partitioned pods are skipped.
@test "min_injected_gomaxprocs skips workload-partitioned pods" {
	cat << EOF > "$CRIO_CONFIG_DIR/01-gomaxprocs.conf"
[crio.runtime]
min_injected_gomaxprocs = 4

[crio.runtime.workloads.management]
activation_annotation = "target.workload.openshift.io/management"
annotation_prefix = "resources.workload.openshift.io"
[crio.runtime.workloads.management.resources]
cpuset = "0-1"
EOF
	start_crio

	jq '.metadata.name = "wp-sandbox"
		| .linux.cgroup_parent = "kubepods-burstable-pod123.slice"
		| .annotations["target.workload.openshift.io/management"] = "{\"effect\":\"PreferredDuringScheduling\"}"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq '.linux.resources.cpu_shares = 2048
		| .linux.resources.cpu_quota = 0' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "not-set" ]]
}

# Verify configurable floor value works.
@test "min_injected_gomaxprocs respects custom floor value" {
	enable_gomaxprocs 8
	start_crio

	create_burstable_sandbox

	# 4096 shares = 4 CPUs. calc=4, floor=8. Should use floor.
	jq '.linux.resources.cpu_shares = 4096
		| .linux.resources.cpu_quota = 0' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(get_gomaxprocs "$ctr_id") == "GOMAXPROCS=8" ]]
}
