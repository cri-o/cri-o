#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	newconfig="$TESTDIR/config.json"
}

function teardown() {
	cleanup_test
}

@test "ctr events ordering for short-lived container" {
	ENABLE_POD_EVENTS=true start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '.command = ["echo", "HELLO"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	wait_for_log "Container event CONTAINER_CREATED_EVENT generated"

	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"

	wait_for_log "Container event CONTAINER_STARTED_EVENT generated for $ctr_id"
	wait_for_log "Container event CONTAINER_STOPPED_EVENT generated for $ctr_id"

	# Extract line numbers from the CRI-O log to verify ordering.
	# STARTED must appear before STOPPED for the same container.
	started_line=$(grep -n "Container event CONTAINER_STARTED_EVENT generated for $ctr_id" "$CRIO_LOG" | head -1 | cut -d: -f1)
	stopped_line=$(grep -n "Container event CONTAINER_STOPPED_EVENT generated for $ctr_id" "$CRIO_LOG" | head -1 | cut -d: -f1)

	echo "STARTED at line $started_line, STOPPED at line $stopped_line"
	[ -n "$started_line" ]
	[ -n "$stopped_line" ]
	[ "$started_line" -lt "$stopped_line" ]

	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "ctr events ordering for multiple short-lived containers" {
	ENABLE_POD_EVENTS=true start_crio

	for i in $(seq 1 5); do
		pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

		jq '.command = ["echo", "RUN'"$i"'"]' \
			"$TESTDATA"/container_config.json > "$newconfig"
		ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)

		crictl start "$ctr_id"
		wait_until_exit "$ctr_id"

		wait_for_log "Container event CONTAINER_STOPPED_EVENT generated for $ctr_id"

		started_line=$(grep -n "Container event CONTAINER_STARTED_EVENT generated for $ctr_id" "$CRIO_LOG" | head -1 | cut -d: -f1)
		stopped_line=$(grep -n "Container event CONTAINER_STOPPED_EVENT generated for $ctr_id" "$CRIO_LOG" | head -1 | cut -d: -f1)

		echo "Run $i: ctr=$ctr_id STARTED at line $started_line, STOPPED at line $stopped_line"
		[ -n "$started_line" ]
		[ -n "$stopped_line" ]
		[ "$started_line" -lt "$stopped_line" ]

		crictl rm "$ctr_id"
		crictl stopp "$pod_id"
		crictl rmp "$pod_id"
	done
}

@test "ctr events ordering for immediate-exit container" {
	ENABLE_POD_EVENTS=true start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '.command = ["true"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)

	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"

	wait_for_log "Container event CONTAINER_STOPPED_EVENT generated for $ctr_id"

	started_line=$(grep -n "Container event CONTAINER_STARTED_EVENT generated for $ctr_id" "$CRIO_LOG" | head -1 | cut -d: -f1)
	stopped_line=$(grep -n "Container event CONTAINER_STOPPED_EVENT generated for $ctr_id" "$CRIO_LOG" | head -1 | cut -d: -f1)

	echo "STARTED at line $started_line, STOPPED at line $stopped_line"
	[ -n "$started_line" ]
	[ -n "$stopped_line" ]
	[ "$started_line" -lt "$stopped_line" ]

	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}
