#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "dedup: startup dedup runs when enabled" {
	cat << EOF > "$CRIO_CONFIG_DIR/01-dedup.conf"
[crio.image]
enable_storage_dedup = true
EOF

	start_crio

	# Verify dedup was triggered at startup
	wait_for_log "Starting storage deduplication"

	# It should complete (either with savings or no savings)
	wait_for_log "Storage deduplication complete"
}

@test "dedup: no dedup when disabled" {
	cat << EOF > "$CRIO_CONFIG_DIR/01-dedup.conf"
[crio.image]
enable_storage_dedup = false
EOF

	start_crio

	# Give it a moment to start
	sleep 2

	# Verify dedup was NOT triggered
	run ! grep -q "Starting storage deduplication" "$CRIO_LOG"
}

@test "dedup: crio dedup command succeeds with expected output" {
	setup_crio

	run "$CRIO_BINARY_PATH" \
		-c "$CRIO_CONFIG" \
		-d "$CRIO_CONFIG_DIR" \
		dedup

	[ "$status" -eq 0 ]
	[[ "$output" == *"Starting storage deduplication"* ]]
	[[ "$output" == *"Storage deduplication complete"* ]]
}

@test "dedup: server remains functional after startup dedup" {
	cat << EOF > "$CRIO_CONFIG_DIR/01-dedup.conf"
[crio.image]
enable_storage_dedup = true
EOF

	start_crio

	wait_for_log "Starting storage deduplication"

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "dedup: config option appears in generated config" {
	output=$("$CRIO_BINARY_PATH" config --default 2> /dev/null)
	[[ "$output" == *"enable_storage_dedup"* ]]
}
