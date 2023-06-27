#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function assert_log() {
	grep -q "Using pull policy path\\s.*\\s$1" "$CRIO_LOG"
}

RESTRICTIVE_POLICY="$INTEGRATION_ROOT/policy-signature.json"

REGISTRY="quay.io/crio"
UNSIGNED_IMAGE="$REGISTRY/unsigned"
SIGNED_IMAGE="$REGISTRY/signed"

SANDBOX_CONFIG="$TESTDATA/sandbox_config.json"

@test "accept unsigned image with default policy" {
	start_crio

	crictl pull "$UNSIGNED_IMAGE"

	assert_log "$SIGNATURE_POLICY"
}

@test "deny unsigned image with restrictive policy" {
	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio

	run ! crictl pull "$UNSIGNED_IMAGE"

	[[ "$output" == *"SignatureValidationFailed"* ]]
	assert_log "$RESTRICTIVE_POLICY"
}

@test "accept signed image with default policy" {
	start_crio

	crictl pull "$SIGNED_IMAGE"

	assert_log "$SIGNATURE_POLICY"
}

@test "accept signed image with restrictive policy" {
	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio

	crictl pull "$SIGNED_IMAGE"

	assert_log "$RESTRICTIVE_POLICY"
}

@test "accept unsigned image with not existing namespace policy" {
	NEW_SANDBOX_CONFIG="$TESTDIR/config.json"
	jq '.metadata.namespace = "foo"' "$SANDBOX_CONFIG" > "$NEW_SANDBOX_CONFIG"

	start_crio

	crictl pull --pod-config "$NEW_SANDBOX_CONFIG" "$UNSIGNED_IMAGE"

	assert_log "$SIGNATURE_POLICY"
}

@test "accept unsigned image with higher priority namespace policy" {
	NEW_SANDBOX_CONFIG="$TESTDIR/config.json"
	jq '.metadata.namespace = "unrestrictive"' "$SANDBOX_CONFIG" > "$NEW_SANDBOX_CONFIG"
	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio

	crictl pull --pod-config "$NEW_SANDBOX_CONFIG" "$UNSIGNED_IMAGE"

	assert_log "$SIGNATURE_POLICY_DIR/unrestrictive.json"
}

@test "deny unsigned image with higher priority namespace policy" {
	NEW_SANDBOX_CONFIG="$TESTDIR/config.json"
	jq '.metadata.namespace = "restrictive"' "$SANDBOX_CONFIG" > "$NEW_SANDBOX_CONFIG"
	start_crio

	run ! crictl pull --pod-config "$NEW_SANDBOX_CONFIG" "$UNSIGNED_IMAGE"

	[[ "$output" == *"SignatureValidationFailed"* ]]
	assert_log "$SIGNATURE_POLICY_DIR/restrictive.json"
}

@test "accept signed image with higher priority namespace policy" {
	NEW_SANDBOX_CONFIG="$TESTDIR/config.json"
	jq '.metadata.namespace = "restrictive"' "$SANDBOX_CONFIG" > "$NEW_SANDBOX_CONFIG"
	start_crio

	crictl pull --pod-config "$NEW_SANDBOX_CONFIG" "$SIGNED_IMAGE"

	assert_log "$SIGNATURE_POLICY_DIR/restrictive.json"
}
