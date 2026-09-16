#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	if [[ -s "$TESTDIR/stop-stacks.log" && -z "${BATS_TEST_COMPLETED:-}" ]]; then
		cp "$TESTDIR/stop-stacks.log" "$ARTIFACTS_PATH/stop-timeout-${CRIO_PID}.stacks"
	fi
	# Always release the simulated hang, including after a failed assertion.
	# SIGKILL terminates the trap below even though it ignores SIGTERM.
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

	# Keep one container alive across stop grace periods by ignoring SIGTERM
	# in PID 1, without an uninterruptible process or an NFS server. A
	# runtime wrapper cannot do this: conmon-rs delivers stop signals
	# out-of-band instead of invoking the runtime binary, so wrapper
	# suppression only works for conmon. Trapping works on both because
	# both transports signal PID 1, and SIGKILL still terminates it below
	# for recovery. Do not exec sleep: PID 1 must stay the trapping shell.
	jq '.command = ["/bin/sh", "-c", "trap '\'''\'' TERM; /bin/sleep 6000"] | .args = []' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container_trap.json"
	pod_id=$(crictl runp "$TESTDATA/sandbox_config.json")
	blocked_ctr=$(crictl create "$pod_id" "$TESTDIR/container_trap.json" "$TESTDATA/sandbox_config.json")
	crictl start "$blocked_ctr"

	# Abort both client requests well before the server's 30s grace period
	# ends. crictl cannot abort itself here: cri-client sets the
	# StopContainer RPC deadline to CRICTL_TIMEOUT + grace, which always
	# outlives a successful server stop and only fires when the server
	# hangs. Use the shell `timeout` to SIGKILL the client after 2s on
	# both runtimes instead. Call $CRICTL_BINARY directly with the same
	# flags as the crictl() helper: `timeout` is an external command and
	# cannot invoke that shell function, and it must kill the real client
	# so the server observes the disconnect.
	# Server-side context error mapping is covered by the server unit tests.
	for _ in 1 2; do
		# `run !` passes on any non-zero status, so a dial failure (exit 1)
		# would vacuously pass like the intended client SIGKILL. Require the
		# client's actual SIGKILL exit instead: timeout kills the still-blocked
		# crictl with 137 only when the server kept the stop open past 2s.
		run timeout -s KILL 2s "$CRICTL_BINARY" -t "$CRICTL_TIMEOUT" --config "$CRICTL_CONFIG_FILE" -r "unix://$CRIO_SOCKET" -i "unix://$CRIO_SOCKET" stop --timeout 30 "$blocked_ctr"
		[ "$status" -eq 137 ]
	done
	[[ $(crictl inspect "$blocked_ctr" | jq -r .status.state) == "CONTAINER_RUNNING" ]]
	crio status --socket "$CRIO_SOCKET" goroutines > "$TESTDIR/stop-stacks.log"
	# Presence before absence: the server must have admitted the stop loop for
	# the still-running container. Neither a WaitOnStopTimeout watcher (removed
	# on cancel) nor a `timed out` log (only after the 30s grace) can prove it.
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

	# A transport deadline alone is not enough: the server must not retain
	# a cleanup goroutine waiting on the unfinished container's state lock.
	retry 20 0.1 stop_cleanup_has_finished

	# Recovery must still allow the original container to stop and clean up:
	# SIGTERM stays trapped, so the 1s grace expires into SIGKILL.
	crictl stop --timeout 1 "$blocked_ctr"
	[[ $(crictl inspect "$blocked_ctr" | jq -r .status.state) == "CONTAINER_EXITED" ]]
	crictl rm "$blocked_ctr"
	blocked_ctr=""
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}
