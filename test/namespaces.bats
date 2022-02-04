#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	export CONTAINER_NAMESPACES_DIR="$TESTDIR"/namespaces
}

function teardown() {
	cleanup_test
}

@test "pid namespace mode pod test" {
	start_crio

	pod_config="$TESTDIR"/sandbox_config.json
	jq '	  .linux.security_context.namespace_options = {
			pid: 0,
			host_network: false,
			host_pid: false,
			host_ipc: false
		}' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"
	pod_id=$(crictl runp "$pod_config")

	ctr_config="$TESTDIR"/config.json
	jq '	  del(.linux.security_context.namespace_options)' \
		"$TESTDATA"/container_redis.json > "$ctr_config"
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" cat /proc/1/cmdline)
	[[ "$output" == *"pause"* ]]
}

@test "pid namespace mode target test" {
	if [[ -v TEST_USERNS ]]; then
		skip "test fails in a user namespace"
	fi
	start_crio

	pod1="$TESTDIR"/sandbox1.json
	jq '	  .linux.security_context.namespace_options = {
			pid: 1,
		}' \
		"$TESTDATA"/sandbox_config.json > "$pod1"
	ctr1="$TESTDIR"/ctr1.json
	jq '	  .linux.security_context.namespace_options = {
			pid: 1,
		}' \
		"$TESTDATA"/container_redis.json > "$ctr1"

	target_ctr=$(crictl run "$ctr1" "$pod1")

	pod2="$TESTDIR"/sandbox2.json
	jq --arg target "$target_ctr" \
		'	  .linux.security_context.namespace_options = {
			pid: 3,
			target_id: $target
		}
		| .metadata.name = "sandbox2" ' \
		"$TESTDATA"/sandbox_config.json > "$pod2"

	ctr2="$TESTDIR"/ctr2.json
	jq --arg target "$target_ctr" \
		'	  .linux.security_context.namespace_options = {
			pid: 3,
			target_id: $target
		}' \
		"$TESTDATA"/container_sleep.json > "$ctr2"

	ctr_id=$(crictl run "$ctr2" "$pod2")

	output1=$(crictl exec --sync "$target_ctr" ps | grep -v ps)
	output2=$(crictl exec --sync "$ctr_id" ps | grep -v ps)
	[[ "$output1" == "$output2" ]]

	crictl rmp -fa
	# make sure namespace is cleaned up
	[[ -z $(ls "$CONTAINER_NAMESPACES_DIR/pidns") ]]
}
