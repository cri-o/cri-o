#!/usr/bin/env bats
# vim:set ft=bash :

# Test for race condition between RunPodSandbox and RemovePodSandbox

load helpers

function setup() {
	setup_test
	export CRIO_LOG_LEVEL=debug

	NRI_PLUGIN_DIR="$TESTDIR/nri-plugins"
	mkdir -p "$NRI_PLUGIN_DIR"
	cp "$NRI_DELAY_PLUGIN_BINARY" "$NRI_PLUGIN_DIR/90-delay-plugin"
	chmod +x "$NRI_PLUGIN_DIR/90-delay-plugin"

	# Configure NRI with extended timeout and plugin directory
	cat << EOF > "$CRIO_CONFIG_DIR/20-nri-timeout.conf"
[crio.nri]
enable_nri = true
nri_plugin_dir = "$NRI_PLUGIN_DIR"
nri_plugin_request_timeout = "30s"
nri_plugin_registration_timeout = "10s"
EOF
}

function teardown() {
	cleanup_test
}

function check_nri_activity() {
	# Verify NRI plugin was invoked by checking that RunPodSandbox took >= 9 seconds
	# The log for NRI plugin is not part of cri-o logs
	local start_time end_time
	start_time=$(grep "Running pod sandbox" "$CRIO_LOG" | tail -1 | awk '{print $1}' | tr -d 'time="' | tr -d 'Z"')
	end_time=$(grep "Ran pod sandbox.*with infra container" "$CRIO_LOG" | tail -1 | awk '{print $1}' | tr -d 'time="' | tr -d 'Z"')

	if [ -n "$start_time" ] && [ -n "$end_time" ]; then
		# Convert timestamps to seconds since epoch using date command
		local start_epoch end_epoch duration
		start_epoch=$(date -d "$start_time" +%s 2> /dev/null || date -j -f "%Y-%m-%dT%H:%M:%S" "${start_time%.*}" +%s 2> /dev/null)
		end_epoch=$(date -d "$end_time" +%s 2> /dev/null || date -j -f "%Y-%m-%dT%H:%M:%S" "${end_time%.*}" +%s 2> /dev/null)
		duration=$((end_epoch - start_epoch))
		[ "$duration" -ge 9 ]
	else
		return 1
	fi
}

function check_delay_plugin() {
	[ -f "$CRIO_LOG" ] && grep -q "delay-plugin\|90-delay-plugin" "$CRIO_LOG"
}

@test "verify NRI delay plugin is loaded and invoked" {
	start_crio
	sleep 2

	# Verify plugin was registered
	check_delay_plugin || {
		echo "# ERROR: NRI delay plugin not found in logs"
		[ -f "$CRIO_LOG" ] && grep -i "plugin\|nri" "$CRIO_LOG" | tail -10
		return 1
	}

	pod_config="$TESTDIR/sandbox_config.json"
	cp "$TESTDATA"/sandbox_config.json "$pod_config"

	pod_id=$(crictl runp "$pod_config")

	check_nri_activity || {
		echo "# ERROR: NRI RunPodSandbox hook not invoked"
		[ -f "$CRIO_LOG" ] && grep -i "nri" "$CRIO_LOG" | tail -5
		return 1
	}

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "pod remove during sandbox creation with NRI delay" {
	start_crio

	pod_config="$TESTDIR/sandbox_config.json"
	cp "$TESTDATA"/sandbox_config.json "$pod_config"

	# Add annotation to set NRI plugin delay to 10 seconds
	jq '.annotations += {"nri-delay-plugin/delay": "10s"}' "$pod_config" > "$pod_config.tmp"
	mv "$pod_config.tmp" "$pod_config"

	# Start pod creation in background - NRI plugin will delay based on annotation
	crictl runp "$pod_config" &
	RUNP_PID=$!

	# Wait for storage creation to complete
	sleep 2

	# Find the pod ID by looking at the most recently created storage directory
	# This works because storage is created before the NRI delay
	# These methods didn't work - Noting down to save someone else's time :)
	# - "sudo crictl pods --state NotReady" - Didn't give the pod id
	# - Tail on the cri-o logs worked but very flaky
	storage_dir="$TESTDIR/crio/overlay-containers"
	pod_id=$(find "$storage_dir" -mindepth 1 -maxdepth 1 -type d -printf '%T@ %f\n' 2> /dev/null | sort -rn | head -1 | awk '{print $2}')

	if [ -z "$pod_id" ]; then
		echo "# ERROR: No pod storage found in $storage_dir"
		return 1
	fi

	# RemovePodSandbox called while RunPodSandbox is in NRI hook (before SetCreated())
	output=$(crictl rmp "$pod_id" 2>&1 || true)

	if [[ "$output" != *"not yet created"* && "$output" != *"sandbox not created"* ]]; then
		echo "# ERROR: Expected 'not yet created' or 'sandbox not created' error during race condition"
		echo "# Got: $output"
		return 1
	fi

	# Wait for RunPodSandbox to complete
	wait $RUNP_PID || true

	# Test idempotent removal
	if crictl pods -q | grep -q "$pod_id"; then
		crictl stopp "$pod_id"
		crictl rmp "$pod_id"
		crictl rmp "$pod_id" || true # Second remove should be idempotent
	fi

	# Verify we can create a new pod after the race condition
	new_pod_id=$(crictl runp "$pod_config")
	crictl stopp "$new_pod_id"
	crictl rmp "$new_pod_id"

	# Verify NRI plugin was actually invoked
	check_nri_activity || echo "# Warning: NRI delay plugin may not have run"
}
