#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
	export CRIO_LOG_LEVEL=debug

	NRI_PLUGIN_DIR="$TESTDIR/nri-plugins"
	mkdir -p "$NRI_PLUGIN_DIR"
	cp "$NRI_DELAY_PLUGIN_BINARY" "$NRI_PLUGIN_DIR/90-delay-plugin"
	chmod +x "$NRI_PLUGIN_DIR/90-delay-plugin"

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

	check_delay_plugin || {
		echo "# ERROR: NRI delay plugin not found in logs"
		[ -f "$CRIO_LOG" ] && grep -i "plugin\|nri" "$CRIO_LOG" | tail -10
		return 1
	}

	pod_config="$TESTDIR/sandbox_config.json"
	nri_log="$TESTDIR/nri-test1.log"
	cp "$TESTDATA"/sandbox_config.json "$pod_config"

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

@test "crictl rmp during sandbox creation returns NotFound via PodSandboxStatus" {
	start_crio

	pod_config="$TESTDIR/sandbox_config_crictl.json"
	nri_log="$TESTDIR/nri-test-crictl.log"

	cp "$TESTDATA"/sandbox_config.json "$pod_config"
	jq '.metadata.name = "test-pod-crictl" | .metadata.uid = "test-uid-crictl"' "$pod_config" > "$pod_config.tmp"
	mv "$pod_config.tmp" "$pod_config"
	jq --arg logfile "$nri_log" '.annotations += {"nri-delay-plugin/delay": "10s", "nri-delay-plugin/log-file": $logfile}' "$pod_config" > "$pod_config.tmp"
	mv "$pod_config.tmp" "$pod_config"

	crictl runp "$pod_config" &
	RUNP_PID=$!

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
		echo "# ERROR: Pod ID not found in NRI log file: $nri_log"
		[ -f "$nri_log" ] && echo "# NRI log contents:" && cat "$nri_log" || echo "# NRI log does not exist"
		wait $RUNP_PID || true
		return 1
	fi

	echo "# Found pod ID: $pod_id"

	output=$(crictl rmp "$pod_id" 2>&1 || true)
	if [[ "$output" != *"code = NotFound"* ]] || [[ "$output" != *"sandbox not created"* ]]; then
		echo "# ERROR: crictl rmp should return NotFound via PodSandboxStatus, got: $output"
		wait $RUNP_PID || true
		return 1
	fi
	echo "# crictl rmp correctly returned NotFound via PodSandboxStatus"

	wait $RUNP_PID || {
		echo "# ERROR: RunPodSandbox failed"
		return 1
	}

	crictl pods -q | grep -q "$pod_id" || {
		echo "# ERROR: Pod missing"
		return 1
	}

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	check_nri_activity "$nri_log"
}

@test "direct gRPC RemovePodSandbox during sandbox creation returns NotFound" {
	start_crio

	pod_config="$TESTDIR/sandbox_config_grpc.json"
	nri_log="$TESTDIR/nri-test-grpc.log"

	cp "$TESTDATA"/sandbox_config.json "$pod_config"
	jq '.metadata.name = "test-pod-grpc" | .metadata.uid = "test-uid-grpc"' "$pod_config" > "$pod_config.tmp"
	mv "$pod_config.tmp" "$pod_config"
	jq --arg logfile "$nri_log" '.annotations += {"nri-delay-plugin/delay": "10s", "nri-delay-plugin/log-file": $logfile}' "$pod_config" > "$pod_config.tmp"
	mv "$pod_config.tmp" "$pod_config"

	crictl runp "$pod_config" &
	RUNP_PID=$!

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
		echo "# ERROR: Pod ID not found in NRI log file: $nri_log"
		[ -f "$nri_log" ] && echo "# NRI log contents:" && cat "$nri_log" || echo "# NRI log does not exist"
		wait $RUNP_PID || true
		return 1
	fi

	echo "# Found pod ID: $pod_id"

	output=$("$CRIOGRPCCALLER_BINARY" remove-pod-sandbox "$CRIO_SOCKET" "$pod_id" 2>&1 || true)
	if [[ "$output" != *"NotFound"* ]] || [[ "$output" != *"not yet created"* ]]; then
		echo "# ERROR: Direct RemovePodSandbox should return NotFound, got: $output"
		wait $RUNP_PID || true
		return 1
	fi
	echo "# Direct RemovePodSandbox correctly returned NotFound"

	wait $RUNP_PID || {
		echo "# ERROR: RunPodSandbox failed"
		return 1
	}

	crictl pods -q | grep -q "$pod_id" || {
		echo "# ERROR: Pod missing"
		return 1
	}

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	check_nri_activity "$nri_log"
}
