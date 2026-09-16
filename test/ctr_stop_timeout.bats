#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	if [[ -s "$TESTDIR/stop-stacks.log" && -z "${BATS_TEST_COMPLETED:-}" ]]; then
		cp "$TESTDIR/stop-stacks.log" "$ARTIFACTS_PATH/stop-timeout-${CRIO_PID}.stacks"
	fi
	# Release the SIGTERM-trapping container even after a failed assertion.
	if [[ -n "${blocked_ctr:-}" ]]; then
		run runtime kill "$blocked_ctr" KILL
	fi
	cleanup_test
}

function stop_cleanup_has_finished() {
	crio status --socket "$CRIO_SOCKET" goroutines > "$TESTDIR/stop-stacks.log" || return 1
	! grep -Fq 'server.(*Server).postStopCleanup(' "$TESTDIR/stop-stacks.log"
}

@test "container stop timeout does not retain cleanup or block other containers" {
	[[ "$RUNTIME_TYPE" == "oci" || "$RUNTIME_TYPE" == "pod" ]] || skip "requires an OCI-backed runtime"

	start_crio

	# Simulate a container that cannot exit: PID 1 ignores SIGTERM. This works
	# with conmon and conmon-rs. Do not exec sleep; PID 1 must stay the shell.
	jq '.command = ["/bin/sh", "-c", "trap '\'''\'' TERM; /bin/sleep 6000"] | .args = []' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container_trap.json"
	pod_id=$(crictl runp "$TESTDATA/sandbox_config.json")
	blocked_ctr=$(crictl create "$pod_id" "$TESTDIR/container_trap.json" "$TESTDATA/sandbox_config.json")
	crictl start "$blocked_ctr"

	# Cancel each stop well before its 30s grace period by killing the client.
	# crictl's own deadline is grace + CRICTL_TIMEOUT, and `timeout` cannot
	# call the crictl() helper, so run the binary with the helper's flags.
	for _ in 1 2; do
		# 137 means the server held the stop open; `run !` would accept a dial failure.
		run timeout -s KILL 2s "$CRICTL_BINARY" -t "$CRICTL_TIMEOUT" --config "$CRICTL_CONFIG_FILE" -r "unix://$CRIO_SOCKET" -i "unix://$CRIO_SOCKET" stop --timeout 30 "$blocked_ctr"
		[ "$status" -eq 137 ]
	done
	[[ $(crictl inspect "$blocked_ctr" | jq -r .status.state) == "CONTAINER_RUNNING" ]]
	crio status --socket "$CRIO_SOCKET" goroutines > "$TESTDIR/stop-stacks.log"
	# The stop loop must still be running before we check for leaked cleanup.
	grep -Fq 'StopLoopForContainer' "$TESTDIR/stop-stacks.log"

	# These operations must remain independent of the unfinished stop.
	jq '.metadata.name = "unrelated-stop-timeout" | .metadata.uid = "unrelated-stop-timeout"' \
		"$TESTDATA/sandbox_config.json" > "$TESTDIR/unrelated-sandbox.json"
	other_pod=$(CRICTL_TIMEOUT=5s crictl runp "$TESTDIR/unrelated-sandbox.json")
	other_ctr=$(CRICTL_TIMEOUT=5s crictl create "$other_pod" "$TESTDATA/container_sleep.json" "$TESTDIR/unrelated-sandbox.json")
	CRICTL_TIMEOUT=5s crictl start "$other_ctr"
	CRICTL_TIMEOUT=5s crictl stop --timeout 1 "$other_ctr"
	CRICTL_TIMEOUT=5s crictl rm "$other_ctr"
	CRICTL_TIMEOUT=5s crictl stopp "$other_pod"
	CRICTL_TIMEOUT=5s crictl rmp "$other_pod"

	# No post-stop cleanup may remain blocked on the running container.
	retry 20 0.1 stop_cleanup_has_finished

	# SIGTERM is trapped, so the 1s grace expires into SIGKILL.
	crictl stop --timeout 1 "$blocked_ctr"
	[[ $(crictl inspect "$blocked_ctr" | jq -r .status.state) == "CONTAINER_EXITED" ]]
	crictl rm "$blocked_ctr"
	blocked_ctr=""
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}
