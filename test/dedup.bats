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

@test "dedup: crio dedup command succeeds" {
	# Setup storage with images but don't start the daemon
	setup_crio

	# Run the dedup command directly
	"$CRIO_BINARY_PATH" \
		-c "$CRIO_CONFIG" \
		-d "$CRIO_CONFIG_DIR" \
		dedup
}

@test "dedup: config option appears in generated config" {
	output=$("$CRIO_BINARY_PATH" config --default 2> /dev/null)
	[[ "$output" == *"enable_storage_dedup"* ]]
}
