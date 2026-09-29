#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	if ! is_cgroup_v2; then
		skip "cgroup mount modes require cgroup v2"
	fi
	if [[ $RUNTIME_TYPE == vm ]]; then
		skip "not applicable to vm runtime type"
	fi
	newconfig="$TESTDIR/config.json"
	sboxconfig="$TESTDIR/sandbox.json"
}

function teardown() {
	cleanup_test
}

# Write a host-network sandbox config with an optional jq filter.
function write_sandbox_config() {
	jq ".linux.security_context.namespace_options.network = 2 ${1:+| $1}" \
		"$TESTDATA/sandbox_config.json" > "$sboxconfig"
}

# Write a host-network container config with the given mode and an optional
# jq filter.
function write_container_config() {
	jq --argjson mode "$1" ".linux.security_context.namespace_options.network = 2 |
		.linux.security_context.cgroup_mount_mode = \$mode ${2:+| $2}" \
		"$TESTDATA/container_sleep.json" > "$newconfig"
}

function assert_cgroup_read_only() {
	run ! crictl exec --sync "$1" mkdir /sys/fs/cgroup/crio-test
	[[ "$output" == *"Read-only file system"* ]]
}

function assert_cgroup_writable() {
	crictl exec --sync "$1" sh -c 'mkdir /sys/fs/cgroup/crio-test && rmdir /sys/fs/cgroup/crio-test'
}

@test "cgroup mount mode read-only rejects privileged containers" {
	start_crio
	write_sandbox_config '.linux.security_context.privileged = true'
	pod_id=$(crictl runp "$sboxconfig")
	write_container_config 1 '.linux.security_context.privileged = true'

	run ! crictl create "$pod_id" "$newconfig" "$sboxconfig"
	[[ "$output" == *"read-only cgroups are not supported for privileged containers"* ]]
}

@test "cgroup mount mode read-only overrides the writable annotation" {
	create_workload_with_allowed_annotation "cgroup2-mount-hierarchy-rw.crio.io"
	start_crio
	# CRI-O ignores the annotation for host-network containers.
	write_sandbox_config '.linux.security_context.namespace_options.network = 0 |
		.annotations."cgroup2-mount-hierarchy-rw.crio.io" = "true"'
	write_container_config 1 '.linux.security_context.namespace_options.network = 0'

	ctr_id=$(crictl run "$newconfig" "$sboxconfig")
	assert_cgroup_read_only "$ctr_id"
}

@test "cgroup mount mode unspecified preserves privileged defaults" {
	start_crio
	write_sandbox_config '.linux.security_context.privileged = true'
	write_container_config 0 '.linux.security_context.privileged = true'

	ctr_id=$(crictl run "$newconfig" "$sboxconfig")
	assert_cgroup_writable "$ctr_id"
}

@test "cgroup mount mode read-only overrides systemd defaults" {
	start_crio
	write_sandbox_config
	pod_id=$(crictl runp "$sboxconfig")
	write_container_config 1 '.command = ["/sbin/init"] | .args = []'

	ctr_id=$(crictl create "$pod_id" "$newconfig" "$sboxconfig")
	crictl inspect "$ctr_id" | jq -e '.info.runtimeSpec.mounts[] |
		select(.destination == "/sys/fs/cgroup") | .options | index("ro") != null'
}
