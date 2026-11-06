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

function write_host_network_sandbox_config() {
	jq ".linux.security_context.namespace_options.network = 2 ${1:+| $1}" \
		"$TESTDATA/sandbox_config.json" > "$sboxconfig"
}

function write_host_network_container_config() {
	jq --argjson mode "$1" ".linux.security_context.namespace_options.network = 2 |
		.linux.security_context.cgroup_mount_mode = \$mode ${2:+| $2}" \
		"$TESTDATA/container_sleep.json" > "$newconfig"
}

function skip_without_nsdelegate() {
	if ! findmnt -n --mountpoint /sys/fs/cgroup -t cgroup2 -O nsdelegate > /dev/null; then
		skip "writable cgroups require nsdelegate"
	fi
}

function assert_cgroup_read_only() {
	run ! crictl exec --sync "$1" mkdir /sys/fs/cgroup/crio-test
	[[ "$output" == *"Read-only file system"* ]]
}

function assert_cgroup_writable() {
	crictl exec --sync "$1" sh -c 'mkdir /sys/fs/cgroup/crio-test && rmdir /sys/fs/cgroup/crio-test'
}

@test "cgroup mount mode read-only rejects privileged containers" {
	# crictl before 1.38 drops cgroup_mount_mode from container configs.
	requires_crictl "1.38"
	start_crio
	write_host_network_sandbox_config '.linux.security_context.privileged = true'
	pod_id=$(crictl runp "$sboxconfig")
	write_host_network_container_config 1 '.linux.security_context.privileged = true'

	run ! crictl create "$pod_id" "$newconfig" "$sboxconfig"
	[[ "$output" == *"read-only cgroups are not supported for privileged containers"* ]]
}

@test "cgroup mount mode read-write confines a container to its cgroup" {
	requires_crictl "1.38"
	skip_without_nsdelegate
	start_crio
	write_host_network_sandbox_config
	write_host_network_container_config 2

	ctr_id=$(crictl run "$newconfig" "$sboxconfig")
	assert_cgroup_writable "$ctr_id"
	output=$(crictl exec --sync "$ctr_id" cat /proc/self/cgroup)
	[[ "$output" == "0::/" ]]
}

@test "cgroup mount mode read-write rejects host-network containers in a user namespace" {
	requires_crictl "1.38"
	skip_without_nsdelegate
	start_crio
	write_host_network_sandbox_config '.linux.security_context.namespace_options.userns_options = {
		"mode": 0,
		"uids": [{"container_id": 0, "host_id": 100000, "length": 65536}],
		"gids": [{"container_id": 0, "host_id": 100000, "length": 65536}]
	}'
	pod_id=$(crictl runp "$sboxconfig")
	write_host_network_container_config 2

	run ! crictl create "$pod_id" "$newconfig" "$sboxconfig"
	[[ "$output" == *"writable cgroups are not supported for host-network containers in a user namespace"* ]]
}

@test "cgroup mount mode read-only overrides the writable annotation" {
	requires_crictl "1.38"
	create_workload_with_allowed_annotation "cgroup2-mount-hierarchy-rw.crio.io"
	start_crio
	# CRI-O ignores the annotation for host-network containers.
	write_host_network_sandbox_config '.linux.security_context.namespace_options.network = 0 |
		.annotations."cgroup2-mount-hierarchy-rw.crio.io" = "true"'
	write_host_network_container_config 1 '.linux.security_context.namespace_options.network = 0'

	ctr_id=$(crictl run "$newconfig" "$sboxconfig")
	assert_cgroup_read_only "$ctr_id"
}

@test "cgroup mount mode unspecified preserves privileged defaults" {
	start_crio
	write_host_network_sandbox_config '.linux.security_context.privileged = true'
	write_host_network_container_config 0 '.linux.security_context.privileged = true'

	ctr_id=$(crictl run "$newconfig" "$sboxconfig")
	assert_cgroup_writable "$ctr_id"
}

@test "cgroup mount mode read-only overrides systemd defaults" {
	requires_crictl "1.38"
	start_crio
	write_host_network_sandbox_config
	pod_id=$(crictl runp "$sboxconfig")
	write_host_network_container_config 1 '.command = ["/sbin/init"] | .args = []'

	ctr_id=$(crictl create "$pod_id" "$newconfig" "$sboxconfig")
	crictl inspect "$ctr_id" | jq -e '.info.runtimeSpec.mounts[] |
		select(.destination == "/sys/fs/cgroup") | .options | index("ro") != null'
}

@test "cgroup mount mode rejects a mount covering /sys/fs/cgroup" {
	requires_crictl "1.38"
	start_crio
	write_host_network_sandbox_config
	pod_id=$(crictl runp "$sboxconfig")
	write_host_network_container_config 1 '.mounts = [{host_path: "/sys/fs", container_path: "/sys/fs"}]'

	run ! crictl create "$pod_id" "$newconfig" "$sboxconfig"
	[[ "$output" == *"the mount at "*"/sys/fs"*" covers /sys/fs/cgroup"* ]]
}
