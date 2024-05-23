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
	grep -q "Using pull policy path\\s.*$1" "$CRIO_LOG"
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

@test "deny unsigned image with restrictive policy if already pulled" {
	start_crio
	crictl pull "$UNSIGNED_IMAGE"
	stop_crio_no_clean

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

@test "allow signed image with restrictive policy on container creation1 (fresh pull)" {
	start_crio
	IMAGE_DIGEST=$(crictl pull "$SIGNED_IMAGE" | cut -d' ' -f7)
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$IMAGE_DIGEST"'" | .image.user_specified_image = "'"$SIGNED_IMAGE"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	# Testing for container start failed not because of the signature, but of
	# the missing command executable
	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"
	[[ "$output" == *"unable to start container process"* || "$output" == *"No such file or directory"* ]]
}

@test "deny unsigned image with restrictive policy on container creation2 (fresh pull)" {
	start_crio
	IMAGE_DIGEST=$(crictl pull "$UNSIGNED_IMAGE" | cut -d' ' -f7)
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$IMAGE_DIGEST"'" | .image.user_specified_image = "'"$UNSIGNED_IMAGE"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"

	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "allow signed image with restrictive policy on container creation3 if already pulled (by ID)" {
	start_crio
	crictl pull "$SIGNED_IMAGE"
	IMAGE_ID=$(crictl images -q "$SIGNED_IMAGE")
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$IMAGE_ID"'" | .image.user_specified_image = "'"$SIGNED_IMAGE"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	# Testing for container start failed not because of the signature, but of
	# the missing command executable
	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"
	[[ "$output" == *"unable to start container process"* || "$output" == *"No such file or directory"* ]]
}

@test "deny unsigned image with restrictive policy on container creation4 if already pulled (by ID)" {
	start_crio
	crictl pull "$UNSIGNED_IMAGE"
	IMAGE_ID=$(crictl images -q "$UNSIGNED_IMAGE")
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$IMAGE_ID"'" | .image.user_specified_image = "'"$UNSIGNED_IMAGE"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"

	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "allow signed image with restrictive policy on container creation5 if already pulled (by tag)" {
	start_crio
	crictl pull "$SIGNED_IMAGE"
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$SIGNED_IMAGE"'" | .image.user_specified_image = "'"$SIGNED_IMAGE"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	# Testing for container start failed not because of the signature, but of
	# the missing command executable
	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"
	[[ "$output" == *"unable to start container process"* || "$output" == *"No such file or directory"* ]]
}

@test "deny unsigned image with restrictive policy on container creation6 if already pulled (by tag)" {
	start_crio
	crictl pull "$UNSIGNED_IMAGE"
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$UNSIGNED_IMAGE"'" | .image.user_specified_image = "'"$UNSIGNED_IMAGE"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"

	[[ "$output" == *"SignatureValidationFailed"* ]]
}
