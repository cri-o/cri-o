#!/usr/bin/env bats
# Integration tests for the layer_dedup config option.
#
# layer_dedup controls whether CRI-O runs a store-wide layer deduplication
# pass (via FIDEDUPERANGE reflinks) after each successful image pull.
# The pass runs in a background worker and never blocks or fails the pull.

load helpers

DEDUP_IMAGE=registry.k8s.io/pause:3.10.2

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

# wait_for_dedup_verdict waits for the outcome of a triggered
# deduplication pass: success once a pass completes, 1 when CRI-O's own
# pre-flight probe reports the filesystem does not support reflink
# deduplication (callers should skip). Basing the decision on the daemon's
# probe keeps the test from disagreeing with CRI-O about support.
function wait_for_dedup_verdict() {
	local cnt=0
	while true; do
		if grep -q "Layer deduplication pass complete" "$CRIO_LOG"; then
			return 0
		fi
		if grep -q "Layer deduplication not supported" "$CRIO_LOG"; then
			return 1
		fi
		# A failure that is not the unsupported-filesystem rejection above
		# is a real error and must fail the test rather than time out.
		if grep -q "Store-wide layer deduplication pass failed" "$CRIO_LOG"; then
			echo "dedup pass failed"
			exit 1
		fi
		# A pass can take minutes on a large store; poll generously.
		if [[ $cnt -gt 600 ]]; then
			echo "dedup pass neither completed nor reported unsupported"
			exit 1
		fi
		sleep 0.1
		cnt=$((cnt + 1))
	done
}

@test "layer_dedup defaults to disabled" {
	# The default config must not enable dedup so that existing deployments
	# are not affected without an explicit opt-in.
	start_crio
	run ! grep -q "Layer dedup worker started" "$CRIO_LOG"
}

@test "layer_dedup disabled does not start the worker" {
	OVERRIDE_OPTIONS="--layer-dedup disabled" start_crio
	run ! grep -q "Layer dedup worker started" "$CRIO_LOG"
}

@test "layer_dedup after_pull starts the worker on startup" {
	OVERRIDE_OPTIONS="--layer-dedup after_pull" start_crio
	wait_for_log "Layer dedup worker started"
}

@test "layer_dedup after_pull triggers a dedup pass after image pull" {
	OVERRIDE_OPTIONS="--layer-dedup after_pull" start_crio

	# Remove the pre-loaded image so we exercise the pull path.
	crictl rmi "$DEDUP_IMAGE" || true
	crictl_pull "$DEDUP_IMAGE"

	# Let CRI-O's own probe decide whether the filesystem supports a pass;
	# skip the test when it does not.
	wait_for_dedup_verdict ||
		skip "CRI-O reports no reflink support on this filesystem"
}

@test "layer_dedup after_pull pull succeeds even when dedup fails on non-reflink filesystem" {
	# This test runs on any filesystem; we only verify that the pull
	# itself never fails due to a dedup error.
	OVERRIDE_OPTIONS="--layer-dedup after_pull" start_crio

	crictl rmi "$DEDUP_IMAGE" || true

	# The pull must succeed regardless of whether the filesystem supports reflinks.
	crictl_pull "$DEDUP_IMAGE"

	# On a reflink-capable filesystem the pass must complete; otherwise the
	# pre-flight probe must reject the pass with a clear warning (the pass
	# itself never starts). Either way the pull succeeded above.
	wait_for_dedup_verdict ||
		grep -q "Layer deduplication not supported" "$CRIO_LOG"

	# Check that no hard error was propagated to the pull response.
	run crictl inspecti "$DEDUP_IMAGE"
	[ "$status" -eq 0 ]
}

@test "layer_dedup after_pull records prometheus metrics on reflink filesystem" {
	local metrics_port
	metrics_port=$(free_port)
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT="$metrics_port" \
		OVERRIDE_OPTIONS="--layer-dedup after_pull" start_crio

	crictl rmi "$DEDUP_IMAGE" || true
	crictl_pull "$DEDUP_IMAGE"

	# Metrics are only recorded by a completed pass, so let CRI-O's own
	# probe decide whether the filesystem supports one.
	wait_for_dedup_verdict ||
		skip "CRI-O reports no reflink support on this filesystem"

	# crictl metricsp queries CRI pod metrics, not the Prometheus HTTP endpoint
	# where the dedup collectors are registered. Query the endpoint directly.
	local metrics
	metrics=$(curl -sf "http://localhost:${metrics_port}/metrics")
	echo "$metrics" | grep -q "crio_image_layer_dedup_duration_seconds"
	echo "$metrics" | grep -q "crio_image_layer_dedup_bytes_saved"
}

@test "layer_dedup after_pull does not queue duplicate passes for concurrent pulls" {
	OVERRIDE_OPTIONS="--layer-dedup after_pull" start_crio

	# Trigger pass 1 and wait until it has actually started so that the
	# concurrent pulls below arrive while the worker is busy.
	crictl pull "$DEDUP_IMAGE" &> /dev/null
	wait_for_log "Starting store-wide layer deduplication pass"

	# Fire N concurrent pulls while pass 1 is in flight. Wait for the
	# pulls by PID: a bare `wait` would also wait for the crio daemon
	# (a background job that only exits at teardown), deadlocking the test.
	local n=5
	local pids=()
	for _ in $(seq 1 $n); do
		crictl pull "$DEDUP_IMAGE" &> /dev/null &
		pids+=("$!")
	done
	wait "${pids[@]}" # wait for all background pulls to finish

	# CRI-O's own probe decides whether passes can run here at all; skip
	# on filesystems without reflink support.
	wait_for_dedup_verdict ||
		skip "CRI-O reports no reflink support on this filesystem"

	# Wait for pass 1 to complete and record its position in the log.
	wait_for_log "Layer deduplication pass complete"
	local after_pass1="$LAST_TIMESTAMP"

	# If the concurrent pulls queued a pass 2, wait for it to complete.
	# Check first so we don't block 25 s on wait_for_log when no pass 2 exists.
	if grep -q "Starting store-wide layer deduplication pass" \
		<(sed -e "1,/$after_pass1/d" < "$CRIO_LOG"); then
		wait_for_log "Layer deduplication pass complete" "$after_pass1"
	fi

	# Assert the serialization invariant: no two passes may run concurrently.
	# Walk the dedup log lines and verify each "Starting…" is followed by its
	# own "complete" before the next "Starting…" appears.
	local in_pass=0 pass_count=0
	while IFS= read -r line; do
		if [[ "$line" == *"Starting store-wide layer deduplication pass"* ]]; then
			[ "$in_pass" -eq 0 ] || {
				echo "BUG: two dedup passes overlapped"
				return 1
			}
			in_pass=1
			pass_count=$((pass_count + 1))
		elif [[ "$line" == *"Layer deduplication pass complete"* ]]; then
			in_pass=0
		fi
	done < <(grep -E "Starting store-wide layer deduplication pass|Layer deduplication pass complete" "$CRIO_LOG")

	# At least one pass must have run.
	[ "$pass_count" -ge 1 ]
}

@test "layer_dedup invalid value is rejected at startup" {
	# CRI-O must refuse to start with an unrecognised layer_dedup value
	# so that typos are caught early rather than silently disabling dedup.
	#
	# NOTE: OVERRIDE_OPTIONS must be set before `run` so bats does not
	# misparse the inline assignment as a command name (which would produce
	# a command-not-found failure instead of a CRI-O validation failure).
	# shellcheck disable=SC2034  # start_crio reads OVERRIDE_OPTIONS internally
	OVERRIDE_OPTIONS="--layer-dedup bogus_value"
	run ! start_crio
	[[ "$output" == *"invalid layer_dedup"* ]]
}

@test "layer_dedup manual starts the worker on startup" {
	OVERRIDE_OPTIONS="--layer-dedup manual" start_crio
	wait_for_log "Layer dedup worker started"
}

@test "layer_dedup manual does not trigger a pass after image pull" {
	OVERRIDE_OPTIONS="--layer-dedup manual" start_crio

	crictl rmi "$DEDUP_IMAGE" || true
	crictl_pull "$DEDUP_IMAGE"

	# Give a wrongly scheduled pass time to appear in the log before
	# asserting that none was triggered.
	sleep 2
	run grep -q "Starting store-wide layer deduplication pass" "$CRIO_LOG"
	[ "$status" -ne 0 ]
}

@test "crio dedup triggers an on-demand pass in manual mode" {
	OVERRIDE_OPTIONS="--layer-dedup manual" start_crio

	run "$CRIO_BINARY_PATH" dedup --socket "$CRIO_SOCKET"
	echo "$output"

	# CRI-O's own pre-flight probe decides whether a pass can run on this
	# filesystem; skip when it cannot. Either way the outcome must be
	# logged server-side, exactly like an after-pull pass.
	if [[ "$output" == *"does not support reflinks"* ]]; then
		wait_for_log "Store-wide layer deduplication pass failed"
		skip "CRI-O reports no reflink support on this filesystem"
	fi

	[ "$status" -eq 0 ]
	[[ "$output" == *"Deduplication pass complete"* ]]
	wait_for_log "Layer deduplication pass complete"
}

@test "crio dedup fails clearly when the worker is not running" {
	OVERRIDE_OPTIONS="--layer-dedup disabled" start_crio

	run "$CRIO_BINARY_PATH" dedup --socket "$CRIO_SOCKET"
	echo "$output"

	[ "$status" -ne 0 ]
	[[ "$output" == *"layer dedup worker is not running"* ]]
}
