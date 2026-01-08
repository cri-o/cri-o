#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "should support valid additional_artifact_stores" {
	# Create a mock additional store directory
	ADDITIONAL_STORE="$TESTDIR/additional-store"
	mkdir -p "$ADDITIONAL_STORE/artifacts"

	# Configure CRI-O to use the additional store using a drop-in config
	cat <<EOF > "$CRIO_CONFIG_DIR/99-artifact.conf"
[crio.runtime]
additional_artifact_stores = [
    "$ADDITIONAL_STORE"
]
EOF

	# Start CRI-O
	start_crio

	# Verify CRI-O started successfully and parsed the config
	run crio-status config
	assert_success
	assert_output --partial 'additional_artifact_stores = ['
	assert_output --partial "$ADDITIONAL_STORE"
}

@test "should fail if additional_artifact_stores path is not absolute" {
	# Configure CRI-O with a relative path
	cat <<EOF > "$CRIO_CONFIG_DIR/99-artifact-invalid.conf"
[crio.runtime]
additional_artifact_stores = [
    "./relative/path/store"
]
EOF

	# Try to start CRI-O, it should fail due to our config validation
	run "$CRIO_BINARY_PATH" --config-dir "$CRIO_CONFIG_DIR" config
	assert_failure
	assert_output --partial "additional_artifact_stores entry must be absolute: \"./relative/path/store\""
}
