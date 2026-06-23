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
	# Verify NRI plugin was invoked by reading its timing log
	local log_file="$1"
	local start_time end_time duration

	[ -f "$log_file" ] || return 1

	start_time=$(grep "^delay_start=" "$log_file" | tail -1 | cut -d= -f2 | tr -d '[:space:]')
	end_time=$(grep "^delay_end=" "$log_file" | tail -1 | cut -d= -f2 | tr -d '[:space:]')

	if [ -n "$start_time" ] && [ -n "$end_time" ]; then
		duration=$((end_time - start_time))
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
	nri_log="$TESTDIR/nri-test1.log"
	cp "$TESTDATA"/sandbox_config.json "$pod_config"

	# Add annotation to write plugin timing to log file
	jq --arg logfile "$nri_log" '.annotations += {"nri-delay-plugin/log-file": $logfile}' "$pod_config" > "$pod_config.tmp"
	mv "$pod_config.tmp" "$pod_config"

	pod_id=$(crictl runp "$pod_config")

	check_nri_activity "$nri_log" || {
		echo "# ERROR: NRI RunPodSandbox hook not invoked"
		[ -f "$nri_log" ] && cat "$nri_log"
		return 1
	}

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "pod remove during sandbox creation with NRI delay" {
	start_crio

	pod_config="$TESTDIR/sandbox_config.json"
	nri_log="$TESTDIR/nri-test2.log"
	cp "$TESTDATA"/sandbox_config.json "$pod_config"

	# Add annotations for delay and log file
	jq --arg logfile "$nri_log" '.annotations += {"nri-delay-plugin/delay": "10s", "nri-delay-plugin/log-file": $logfile}' "$pod_config" > "$pod_config.tmp"
	mv "$pod_config.tmp" "$pod_config"

	# Start pod creation in background - NRI plugin will delay based on annotation
	crictl runp "$pod_config" &
	RUNP_PID=$!

	# Wait for NRI plugin to write pod ID to log file
	local retry=0
	pod_id=""
	while [ $retry -lt 20 ]; do
		if [ -f "$nri_log" ]; then
			pod_id=$(grep "^pod_id=" "$nri_log" | tail -1 | cut -d= -f2 | tr -d '[:space:]')
			[ -n "$pod_id" ] && break
		fi
		sleep 0.5
		retry=$((retry + 1))
	done

	if [ -z "$pod_id" ]; then
		echo "# ERROR: Pod ID not found in NRI plugin log"
		[ -f "$nri_log" ] && cat "$nri_log"
		return 1
	fi

	echo "# Found pod ID from NRI plugin: $pod_id"

	# RemovePodSandbox called while RunPodSandbox is in NRI hook (before SetCreated())
	# Expected: gRPC NotFound error (pod exists in storage but not yet marked as created)
	output=$(crictl rmp "$pod_id" 2>&1 || true)

	if [[ "$output" != *"code = NotFound"* ]]; then
		echo "# ERROR: Expected gRPC NotFound error during race condition"
		echo "# Got: $output"
		return 1
	fi

	echo "# Race condition correctly triggered: NotFound error received"

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
	check_nri_activity "$nri_log"
}
