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

# skip_unless_reflink_supported skips the test if the container storage root
# is on a filesystem that does not support reflinks (XFS with reflink=1 or Btrfs).
function skip_unless_reflink_supported() {
	local testfile
	testfile=$(mktemp "$TESTDIR"/reflink-check.XXXXXX)
	if ! cp --reflink=always "$testfile" "${testfile}.reflink" 2> /dev/null; then
		rm -f "$testfile" "${testfile}.reflink"
		skip "filesystem does not support reflinks (XFS with reflink=1 or Btrfs required)"
	fi
	rm -f "$testfile" "${testfile}.reflink"
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

	wait_for_log "Starting store-wide layer deduplication pass"
	wait_for_log "Layer deduplication pass complete"
}

@test "layer_dedup after_pull pull succeeds even when dedup fails on non-reflink filesystem" {
	# This test runs on any filesystem; we only verify that the pull
	# itself never fails due to a dedup error.
	OVERRIDE_OPTIONS="--layer-dedup after_pull" start_crio

	crictl rmi "$DEDUP_IMAGE" || true

	# The pull must succeed regardless of whether the filesystem supports reflinks.
	crictl_pull "$DEDUP_IMAGE"

	# Dedup either completed cleanly or logged a warning — either way the
	# pull returned successfully (validated by crictl_pull exit code above).
	# Check that no hard error was propagated to the pull response.
	run crictl inspecti "$DEDUP_IMAGE"
	[ "$status" -eq 0 ]
}

@test "layer_dedup after_pull records prometheus metrics on reflink filesystem" {
	skip_unless_reflink_supported

	local metrics_port
	metrics_port=$(free_port)
	CONTAINER_ENABLE_METRICS="true" CONTAINER_METRICS_PORT="$metrics_port" \
		OVERRIDE_OPTIONS="--layer-dedup after_pull" start_crio

	crictl rmi "$DEDUP_IMAGE" || true
	crictl_pull "$DEDUP_IMAGE"

	wait_for_log "Layer deduplication pass complete"

	# crictl metricsp queries CRI pod metrics, not the Prometheus HTTP endpoint
	# where the dedup collectors are registered. Query the endpoint directly.
	local metrics
	metrics=$(curl -sf "http://localhost:${metrics_port}/metrics")
	echo "$metrics" | grep -q "crio_image_layer_dedup_duration_seconds"
	echo "$metrics" | grep -q "crio_image_layer_dedup_bytes_saved"
}

@test "layer_dedup after_pull does not queue duplicate passes for concurrent pulls" {
	skip_unless_reflink_supported

	OVERRIDE_OPTIONS="--layer-dedup after_pull" start_crio

	# Trigger pass 1 and wait until it has actually started so that the
	# concurrent pulls below arrive while the worker is busy.
	crictl pull "$DEDUP_IMAGE" &> /dev/null
	wait_for_log "Starting store-wide layer deduplication pass"

	# Fire N concurrent pulls while pass 1 is in flight.
	local n=5
	for _ in $(seq 1 $n); do
		crictl pull "$DEDUP_IMAGE" &> /dev/null &
	done
	wait # wait for all background pulls to finish

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
