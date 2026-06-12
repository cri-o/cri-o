#!/usr/bin/env bats

# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "ensure correct hostname" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec --sync "$ctr_id" sh -c "hostname")
	[[ "$output" == *"crictl_host"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "echo \$HOSTNAME")
	[[ "$output" == *"crictl_host"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "cat /etc/hostname")
	[[ "$output" == *"crictl_host"* ]]
}

@test "ensure correct hostname for hostnetwork:true" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	start_crio

	jq '	  .linux.security_context.namespace_options.network = 2
		| del(.annotations)
		| del(.hostname)' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_hostnetwork.json

	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox_hostnetwork.json)

	output=$(crictl exec --sync "$ctr_id" sh -c "hostname")
	[[ "$output" == *"$HOSTNAME"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "echo \$HOSTNAME")
	[[ "$output" == *"$HOSTNAME"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "cat /etc/hostname")
	[[ "$output" == *"$HOSTNAME"* ]]
}

@test "Check for valid pod netns CIDR" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec --sync "$ctr_id" ip addr show dev eth0 scope global)
	[[ "$output" = *" inet $POD_IPV4_CIDR_START"* ]]
	[[ "$output" = *" inet6 $POD_IPV6_CIDR_START"* ]]
}

@test "Ensure correct CNI plugin namespace/name/container-id arguments" {
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}" CNI_TYPE="cni_plugin_helper.bash" start_crio

	crictl runp "$TESTDATA"/sandbox_config.json

	# shellcheck disable=SC1090,SC1091
	. "$TESTDIR"/plugin_test_args.out

	[ "$FOUND_CNI_CONTAINERID" != "redhat.test.crio" ]
	[ "$FOUND_CNI_CONTAINERID" != "podsandbox1" ]
	[ "$FOUND_K8S_POD_NAMESPACE" = "redhat.test.crio" ]
	[ "$FOUND_K8S_POD_NAME" = "podsandbox1" ]
	[ "$FOUND_K8S_POD_UID" = "redhat-test-crio" ]
}

@test "Connect to pod hostport from the host" {
	if is_cgroup_v2; then
		skip "node configured with cgroupv2 flakes this test sometimes"
	fi
	start_crio

	pod_config="$TESTDIR"/sandbox_config.json
	jq '	  .port_mappings = [ {
			protocol: 0,
			container_port: 80,
			host_port: 4888
		} ]
		| .hostname = "very.unique.name" ' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"

	ctr_config="$TESTDIR"/container_config.json
	jq '	  .command = [ "/bin/nc", "-ll", "-p", "80", "-e", "/bin/hostname" ]' \
		"$TESTDATA"/container_config.json > "$ctr_config"

	crictl run "$ctr_config" "$pod_config"

	host_ip=$(get_host_ip)
	output=$(nc -w 5 "$host_ip" 4888 < /dev/null)
	[ "$output" = "very.unique.name" ]
}

# ensure that the server cleaned up sandbox networking
# if the sandbox failed after network setup
function check_networking() {
	# shellcheck disable=SC2010
	if ls /var/lib/cni/networks/"$CNI_DEFAULT_NETWORK" | grep -Ev '^lock|^last_reserved_ip'; then
		echo "unexpected networks found" 1>&2
		exit 1
	fi
}

@test "Clean up network if pod sandbox fails" {
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

	# TODO FIXME find a way for sandbox setup to fail if manage ns is true
	CONMON_BINARY="$TESTDIR"/conmon
	cp "$(command -v conmon)" "$CONMON_BINARY"
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}"
	CONTAINER_DROP_INFRA_CTR=false start_crio

	# make conmon non-executable to cause the sandbox setup to fail after
	# networking has been configured
	chmod 0644 "$CONMON_BINARY"
	run ! crictl runp "$TESTDATA"/sandbox_config.json

	check_networking
}

@test "Clean up network if pod sandbox fails after plugin success" {
	# shellcheck disable=SC2030
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}"
	CNI_TYPE="cni_plugin_helper.bash" setup_crio
	echo "DEBUG_ARGS=malformed-result" > "$TESTDIR"/cni_plugin_helper_input.env
	start_crio_no_setup
	check_images

	run ! crictl runp "$TESTDATA"/sandbox_config.json

	check_networking
}

@test "Clean up network if pod sandbox gets killed" {
	CONTAINER_DROP_INFRA_CTR=false start_crio

	CNI_RESULTS_DIR=/var/lib/cni/results
	POD=$(crictl runp "$TESTDATA/sandbox_config.json")

	# CNI result is there
	# shellcheck disable=SC2010
	[[ $(ls $CNI_RESULTS_DIR | grep "$POD") != "" ]]

	# kill the sandbox
	runtime kill "$POD" KILL

	# wait for the pod to be killed
	while crictl inspectp "$POD" | jq -e '.status.state != "SANDBOX_NOTREADY"' > /dev/null; do
		echo Waiting for sandbox to be stopped
	done

	# now remove the sandbox
	crictl rmp "$POD"

	# CNI result is gone
	# shellcheck disable=SC2010
	[[ $(ls $CNI_RESULTS_DIR | grep "$POD") == "" ]]
}

@test "Clean up network if pod netns gets destroyed" {
	start_crio

	POD=$(crictl runp "$TESTDATA/sandbox_config.json")

	# remove the network namespace
	NETNS_PATH=/var/run/netns/
	NS=$(crictl inspectp "$POD" |
		jq -er '.info.runtimeSpec.linux.namespaces[] | select(.type == "network").path | sub("'$NETNS_PATH'"; "")')

	# remove network namespace
	ip netns del "$NS"

	# fake invalid netns path
	touch "$NETNS_PATH$NS"

	# be able to remove the sandbox
	crictl rmp -f "$POD"
	grep -q "Removed netns path $NETNS_PATH$NS from pod sandbox" "$CRIO_LOG"
}

@test "Network recovery after reboot with destroyed netns" {
	# This test simulates a reboot scenario where network namespaces are destroyed
	# but CRI-O needs to clean up pod network resources gracefully.

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Get the network namespace path
	NETNS_PATH=/var/run/netns/
	NS=$(crictl inspectp "$pod_id" |
		jq -er '.info.runtimeSpec.linux.namespaces[] | select(.type == "network").path | sub("'$NETNS_PATH'"; "")')

	# Remove the network namespace.
	ip netns del "$NS"

	# Create a fake netns file.
	touch "$NETNS_PATH$NS"

	restart_crio

	# Try to remove the pod.
	crictl rmp -f "$pod_id" 2> /dev/null || true

	grep -q "Successfully cleaned up network for pod" "$CRIO_LOG"

	new_pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Verify the new pod is running.
	output=$(crictl inspectp "$new_pod_id" | jq -r '.status.state')
	[[ "$output" == "SANDBOX_READY" ]]

	# Clean up the new pod
	crictl stopp "$new_pod_id"
	crictl rmp "$new_pod_id"
}

@test "CNI teardown called even with missing or invalid netns to prevent IP leaks" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Get the network namespace path
	NETNS_PATH=/var/run/netns/
	NS=$(crictl inspectp "$pod_id" |
		jq -er '.info.runtimeSpec.linux.namespaces[] | select(.type == "network").path | sub("'$NETNS_PATH'"; "")')

	output=$(crictl inspectp "$pod_id" | jq -r '.status.state')
	[[ "$output" == "SANDBOX_READY" ]]

	# Stop the pod first to release the network namespace.
	crictl stopp "$pod_id"

	# Now remove the network namespace file to simulate the issue.
	rm -f "$NETNS_PATH$NS"

	# Remove the pod - this should still call CNI teardown.
	crictl rmp "$pod_id"

	# Verify CNI teardown was called by checking logs.
	grep -q "Deleting pod.*from CNI network" "$CRIO_LOG"

	# Verify the pod is gone.
	run ! crictl inspectp "$pod_id"
}

@test "pod deletion succeeds when NetNS path is missing" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	crictl ps --id "$ctr_id" | grep Running

	netns_path=$(crictl inspectp "$pod_id" | jq -r '.status.network.namespace_path')

	# Simulating the scenario where infra container dies and NetNS becomes invalid
	# by removing the network namespace file.
	if [[ -n "$netns_path" && -f "$netns_path" ]]; then
		rm -f "$netns_path"
	fi

	crictl stop "$ctr_id"
	crictl rm "$ctr_id"

	crictl stopp "$pod_id"

	# Pod deletion should succeed even with missing NetNS
	crictl rmp "$pod_id"

	run ! crictl inspectp "$pod_id"
}

@test "netns file cleanup after CRI-O restart with invalid namespace" {
	# This test reproduces the bug where invalid netns files are not cleaned up
	# after CRI-O restart, preventing pods from restarting.
	start_crio

	# Create and run a pod
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Get the network namespace path
	NETNS_PATH=/var/run/netns/
	NS=$(crictl inspectp "$pod_id" |
		jq -er '.info.runtimeSpec.linux.namespaces[] | select(.type == "network").path | sub("'$NETNS_PATH'"; "")')

	echo "Pod $pod_id created with netns: $NETNS_PATH$NS"

	# Verify the netns file exists before stopping
	[[ -f "$NETNS_PATH$NS" ]] || [[ -f "/run/netns/$NS" ]]

	# Stop the pod (this marks it as stopped in CRI-O state)
	crictl stopp "$pod_id"

	# After stopping, the namespace might already be cleaned up by CRI-O
	# We need to recreate the invalid file scenario that happens after reboot
	# where the kernel namespace is gone but the file remains

	# Try to delete the kernel namespace if it still exists (may fail if already gone)
	ip netns del "$NS" 2> /dev/null || true

	# Create a fake/invalid netns file (kernel namespace is gone but file exists)
	touch "$NETNS_PATH$NS" || touch "/run/netns/$NS"

	# Use whichever path actually exists
	if [[ -f "/run/netns/$NS" ]]; then
		NETNS_PATH=/run/netns/
	fi

	# Verify the invalid file exists
	[[ -f "$NETNS_PATH$NS" ]]
	echo "Invalid netns file created at: $NETNS_PATH$NS"
	ls -la "$NETNS_PATH$NS" || true

	# Restart CRI-O - this triggers LoadSandbox which calls NetNsJoin
	# NetNsJoin should fail to GetNS but will store partial namespace anyway
	restart_crio

	echo "After CRI-O restart, checking if invalid file still exists..."
	if [[ -f "$NETNS_PATH$NS" ]]; then
		echo "BUG CONFIRMED: Invalid netns file still exists: $NETNS_PATH$NS"
		ls -la "$NETNS_PATH$NS"
	else
		echo "File was cleaned up (bug may be fixed)"
	fi

	# Try to remove the pod - this should cleanup the invalid file
	crictl rmp -f "$pod_id"

	echo "After pod removal, verifying file cleanup..."
	if [[ -f "$NETNS_PATH$NS" ]]; then
		echo "LEAKED: Invalid netns file was not cleaned up: $NETNS_PATH$NS"
		ls -la "$NETNS_PATH$NS" || true
		# Fail the test - this demonstrates the bug
		echo "TEST FAILED: Bug reproduced - netns file leaked"
		return 1
	else
		echo "SUCCESS: Invalid netns file was properly cleaned up"
	fi

	# Verify we can create a new pod now (would fail with leaked file)
	new_pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	output=$(crictl inspectp "$new_pod_id" | jq -r '.status.state')
	[[ "$output" == "SANDBOX_READY" ]]
}

# Wait for a log message with a configurable timeout (default 15s).
# The built-in wait_for_log has a fixed ~5s timeout which is too short
# for CNI STATUS monitoring (5s poll interval means up to 10s for events).
function wait_for_cni_log() {
	local pattern="$1"
	local max_wait="${2:-150}" # 150 * 0.1s = 15s default
	local cnt=0
	while ! grep -q "$pattern" "$CRIO_LOG" 2> /dev/null; do
		if [[ $cnt -gt $max_wait ]]; then
			echo "timed out waiting for: $pattern"
			cat "$CRIO_LOG"
			exit 1
		fi
		sleep 0.1
		cnt=$((cnt + 1))
	done
}

# Helper to write a CNI 1.1.0 conflist that enables STATUS support.
# shellcheck disable=SC2031
function prepare_network_conf_with_status() {
	mkdir -p "$CRIO_CNI_CONFIG"
	cat > "$CRIO_CNI_CONFIG/10-crio.conflist" <<- EOF
		{
		    "cniVersion": "1.1.0",
		    "name": "$CNI_DEFAULT_NETWORK",
		    "disableGC": true,
		    "plugins": [
		        {
		            "cniVersion": "1.1.0",
		            "name": "$CNI_DEFAULT_NETWORK",
		            "type": "$CNI_TYPE",
		            "bridge": "cni0",
		            "isGateway": true,
		            "ipMasq": true,
		            "ipam": {
		                "type": "host-local",
		                "routes": [
		                    { "dst": "$POD_IPV4_DEF_ROUTE" },
		                    { "dst": "$POD_IPV6_DEF_ROUTE" }
		                ],
		                "ranges": [
		                    [{ "subnet": "$POD_IPV4_CIDR" }],
		                    [{ "subnet": "$POD_IPV6_CIDR" }]
		                ]
		            }
		        }
		    ]
		}
	EOF
}

@test "CNI status grace period tolerates transient failure" {
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}"
	CNI_TYPE="cni_plugin_helper.bash"
	setup_crio

	# Use CNI 1.1.0 conflist to enable STATUS command
	prepare_network_conf_with_status

	# Set a grace period long enough to survive the transient failure
	cat << EOF > "$CRIO_CONFIG_DIR/01-cni-grace.conf"
[crio.network]
cni_status_grace_period = "30s"
EOF

	echo "DEBUG_ARGS=" > "$TESTDIR"/cni_plugin_helper_input.env
	start_crio_no_setup
	check_images

	wait_for_cni_log "Continuous CNI STATUS monitoring enabled"

	# Trigger a transient CNI failure
	touch "$TESTDIR/cni_plugin_status_failing"
	wait_for_cni_log "CNI plugin status check failed, will report unhealthy after grace period"

	# Remove the failure before grace period expires
	rm -f "$TESTDIR/cni_plugin_status_failing"

	# Allow a couple of poll cycles for the plugin to be re-checked
	sleep 2

	# Node should still be network-ready (grace period was not exceeded)
	output=$(crictl info -o json | jq -r '.status.conditions[] | select(.type == "NetworkReady") | .status')
	[[ "$output" == "true" ]]
}

@test "CNI status grace period reports unhealthy after expiry" {
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}"
	CNI_TYPE="cni_plugin_helper.bash"
	setup_crio

	# Use CNI 1.1.0 conflist to enable STATUS command
	prepare_network_conf_with_status

	# Set a very short grace period so the test doesn't take long
	cat << EOF > "$CRIO_CONFIG_DIR/01-cni-grace.conf"
[crio.network]
cni_status_grace_period = "1s"
EOF

	echo "DEBUG_ARGS=" > "$TESTDIR"/cni_plugin_helper_input.env
	start_crio_no_setup
	check_images

	wait_for_cni_log "Continuous CNI STATUS monitoring enabled"

	# Trigger a persistent CNI failure and wait for the grace period to expire.
	# The monitor polls every 5s, so the "beyond grace period" log needs 2 polls
	# (~10s): first poll sets the timer, second poll finds grace (1s) expired.
	touch "$TESTDIR/cni_plugin_status_failing"
	wait_for_cni_log "CNI plugin unhealthy beyond grace period"

	# Node should report network not ready
	output=$(crictl info -o json | jq -r '.status.conditions[] | select(.type == "NetworkReady") | .status')
	[[ "$output" == "false" ]]
}

@test "CNI status monitoring disabled when grace period is zero" {
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}"
	CNI_TYPE="cni_plugin_helper.bash"
	setup_crio

	# Use CNI 1.1.0 conflist to enable STATUS command
	prepare_network_conf_with_status

	# Default grace period is 0 -- monitoring should be disabled
	echo "DEBUG_ARGS=" > "$TESTDIR"/cni_plugin_helper_input.env
	start_crio_no_setup
	check_images

	wait_for_cni_log "Continuous CNI STATUS monitoring is disabled"

	# Even with a failing plugin, node should still report ready
	touch "$TESTDIR/cni_plugin_status_failing"
	sleep 2

	output=$(crictl info -o json | jq -r '.status.conditions[] | select(.type == "NetworkReady") | .status')
	[[ "$output" == "true" ]]
}
