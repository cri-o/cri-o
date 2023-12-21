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
	if [[ "$TEST_USERNS" == "1" ]]; then
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

@test "KUBENSMNT mount namespace" {
	original_ns=$(readlink /proc/self/ns/mnt)

	PIN_ROOT=$TESTDIR/kubens
	mkdir -p "$PIN_ROOT"

	# Set up a pinned mount namespace
	$PINNS_BINARY_PATH -d "$PIN_ROOT" -f mnt -m
	PINNED_MNT_NS=$PIN_ROOT/mntns/mnt
	# Ensure it pinned a new unique mount namespace
	pinned_ns=$(nsenter -m"$PINNED_MNT_NS" readlink /proc/self/ns/mnt)
	[[ "$pinned_ns" != "$original_ns" ]]

	# First test: No environment set; no namespace should be joined
	start_crio
	# Ensure CRI-O is running in the original namespace
	[[ -n $CRIO_PID ]]
	crio_ns=$(readlink /proc/"$CRIO_PID"/ns/mnt)
	[[ "$crio_ns" == "$original_ns" ]]
	stop_crio

	# Positive test: Join the right namespace
	export KUBENSMNT=$PINNED_MNT_NS
	start_crio
	# Ensure CRI-O is running in the specified pinned namespace
	[[ -n $CRIO_PID ]]
	crio_ns=$(readlink /proc/"$CRIO_PID"/ns/mnt)
	[[ "$crio_ns" == "$pinned_ns" ]]
	stop_crio

	# Negative test: Set KUBENSMNT to a nonexistent file
	export KUBENSMNT=$PIN_ROOT/nosuchfile
	start_crio
	# Ensure CRI-O is running in the original namespace
	[[ -n $CRIO_PID ]]
	crio_ns=$(readlink /proc/"$CRIO_PID"/ns/mnt)
	[[ "$crio_ns" == "$original_ns" ]]
	stop_crio

	# Negative test: set KUBENSMNT to a valid file that is NOT a bindmount
	export KUBENSMNT=$PIN_ROOT/not_a_bindmount
	touch "$KUBENSMNT"
	start_crio
	# Ensure CRI-O is running in the original namespace
	[[ -n $CRIO_PID ]]
	crio_ns=$(readlink /proc/"$CRIO_PID"/ns/mnt)
	[[ "$crio_ns" == "$original_ns" ]]
	stop_crio

	# Negative test: set KUBENSMNT to a valid bindmount that is NOT a mount namespace
	$PINNS_BINARY_PATH -d "$PIN_ROOT" -f net -n
	PINNED_NET_NS=$PIN_ROOT/netns/net
	export KUBENSMNT=$PINNED_NET_NS
	start_crio
	# Ensure CRI-O is running in the original namespace
	[[ -n $CRIO_PID ]]
	crio_ns=$(readlink /proc/"$CRIO_PID"/ns/mnt)
	[[ "$crio_ns" == "$original_ns" ]]
	stop_crio
}
