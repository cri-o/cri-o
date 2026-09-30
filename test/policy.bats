#!/usr/bin/env bats
# vim:set ft=bash :

# TODO(bitoku): These tests require test/default.yaml to be in /etc/containers/registries.d/default.yaml
# Add check to ensure it.

load helpers

function setup() {
	setup_test
}

function teardown() {
	stop_registry
	cleanup_test
}

function assert_log() {
	grep -q "Using pull policy path\\s.*$1" "$CRIO_LOG"
}

RESTRICTIVE_POLICY="$INTEGRATION_ROOT/policy-signature.json"

CONTAINER_PATH=/volume
REGISTRY="quay.io/crio"
UNSIGNED_IMAGE="$REGISTRY/unsigned"
SIGNED_IMAGE="$REGISTRY/signed"

SANDBOX_CONFIG="$TESTDATA/sandbox_config.json"

MLDSA_IMAGE="mldsa/pause:latest"

# Start a test registry with $1 certificates, push $MLDSA_IMAGE to it and
# generate an image signing key pair $TESTDIR/signing.{key,pub} using $1.
function setup_mldsa_image() {
	start_registry "$1"
	push_to_registry "$MLDSA_IMAGE"
	"$REGISTRY_BINARY" generate-key --algorithm "$1" --prefix "$TESTDIR/signing"
}

# Pull $MLDSA_IMAGE and verify that all TLS handshakes it caused used TLS 1.3,
# hybrid ML-KEM key exchange and a verified $1 client certificate. CRI-O does
# not configure key exchanges for registry connections, so this relies on Go
# preferring X25519MLKEM768 by default.
function pull_mldsa_image_over_pqc_tls() {
	local log_lines handshakes
	log_lines=$(wc -l < "$REGISTRY_LOG")

	crictl_pull "$REGISTRY_ADDRESS/$MLDSA_IMAGE"

	handshakes=$(tail -n +"$((log_lines + 1))" "$REGISTRY_LOG" | grep "TLS handshake")
	[ -n "$handshakes" ]
	while read -r line; do
		[[ "$line" == *"version=TLS 1.3 "* ]]
		[[ "$line" == *"key-exchange=X25519MLKEM768 "* ]]
		[[ "$line" == *"client-certificate=$1 "* ]]
		[[ "$line" == *"verified-client-chains=1"* ]]
	done <<< "$handshakes"
}

# Write $MLDSA_POLICY, which requires images of the test registry to be signed with
# public key $1, and start CRI-O with it.
function start_crio_with_mldsa_policy() {
	MLDSA_POLICY="$TESTDIR/policy.json"
	jq -n --arg scope "$REGISTRY_ADDRESS/${MLDSA_IMAGE%:*}" --arg key "$1" \
		'{default: [{type: "reject"}], transports: {docker: {($scope): [
			{type: "sigstoreSigned", keyPath: $key, signedIdentity: {type: "matchRepository"}}
		]}}}' > "$MLDSA_POLICY"
	SIGNATURE_POLICY="$MLDSA_POLICY" start_crio
}

@test "accept unsigned image with default policy" {
	start_crio

	crictl_pull "$UNSIGNED_IMAGE"

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
	crictl_pull "$UNSIGNED_IMAGE"
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	run ! crictl pull "$UNSIGNED_IMAGE"

	[[ "$output" == *"SignatureValidationFailed"* ]]
	assert_log "$RESTRICTIVE_POLICY"
}

@test "accept signed image with default policy" {
	start_crio

	crictl_pull "$SIGNED_IMAGE"

	assert_log "$SIGNATURE_POLICY"
}

@test "accept signed image with restrictive policy" {
	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio

	crictl_pull "$SIGNED_IMAGE"

	assert_log "$RESTRICTIVE_POLICY"
}

@test "deny signed image with invalid policy (subjectEmail)" {
	POLICY="$TESTDIR/policy.json"
	jq '.transports.docker["'"$SIGNED_IMAGE"'"][0].fulcio.subjectEmail = "invalid"' "$RESTRICTIVE_POLICY" > "$POLICY"
	SIGNATURE_POLICY="$POLICY" start_crio

	run ! crictl pull "$SIGNED_IMAGE"

	[[ "$output" == *"SignatureValidationFailed"* ]]
	assert_log "$POLICY"
}

@test "deny signed image with invalid policy (oidcIssuer)" {
	POLICY="$TESTDIR/policy.json"
	jq '.transports.docker["'"$SIGNED_IMAGE"'"][0].fulcio.oidcIssuer = "invalid"' "$RESTRICTIVE_POLICY" > "$POLICY"
	SIGNATURE_POLICY="$POLICY" start_crio

	run ! crictl pull "$SIGNED_IMAGE"

	[[ "$output" == *"SignatureValidationFailed"* ]]
	assert_log "$POLICY"
}

@test "accept unsigned image with not existing namespace policy" {
	NEW_SANDBOX_CONFIG="$TESTDIR/config.json"
	jq '.metadata.namespace = "foo"' "$SANDBOX_CONFIG" > "$NEW_SANDBOX_CONFIG"

	start_crio

	crictl_pull --pod-config "$NEW_SANDBOX_CONFIG" "$UNSIGNED_IMAGE"

	assert_log "$SIGNATURE_POLICY"
}

@test "accept unsigned image with higher priority namespace policy" {
	NEW_SANDBOX_CONFIG="$TESTDIR/config.json"
	jq '.metadata.namespace = "unrestrictive"' "$SANDBOX_CONFIG" > "$NEW_SANDBOX_CONFIG"
	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio

	crictl_pull --pod-config "$NEW_SANDBOX_CONFIG" "$UNSIGNED_IMAGE"

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

	crictl_pull --pod-config "$NEW_SANDBOX_CONFIG" "$SIGNED_IMAGE"

	assert_log "$SIGNATURE_POLICY_DIR/restrictive.json"
}

@test "allow signed image with restrictive policy on container creation1 (fresh pull)" {
	start_crio
	# Pull returns the image ID directly now, use it to verify the exact data flow
	# from PullImage to CreateContainer
	IMAGE_ID=$(crictl_pull "$SIGNED_IMAGE" | awk '{print $NF}')
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

@test "deny unsigned image with restrictive policy on container creation2 (fresh pull)" {
	start_crio
	# Pull returns the image ID directly now, use it to verify the exact data flow
	# from PullImage to CreateContainer
	IMAGE_ID=$(crictl_pull "$UNSIGNED_IMAGE" | awk '{print $NF}')
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$IMAGE_ID"'" | .image.user_specified_image = "'"$UNSIGNED_IMAGE"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"

	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "allow signed image with restrictive policy on container creation3 if already pulled (by ID)" {
	start_crio
	crictl_pull "$SIGNED_IMAGE"
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
	crictl_pull "$UNSIGNED_IMAGE"
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
	crictl_pull "$SIGNED_IMAGE"
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
	crictl_pull "$UNSIGNED_IMAGE"
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$UNSIGNED_IMAGE"'" | .image.user_specified_image = "'"$UNSIGNED_IMAGE"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"

	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "allow signed image with restrictive policy on container creation7 if already pulled (by tag and ID)" {
	start_crio
	crictl_pull "$SIGNED_IMAGE"
	# Insert "latest" tag into the repoDigests field, and use that as the reference
	# CRI-O should filter out the :latest bit, so it's a valid reference for c/image
	REPO_TAG_DIGEST=$(crictl inspecti "$SIGNED_IMAGE" | jq -r .status.repoDigests[0] | sed "s|@|:latest@|g")
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$REPO_TAG_DIGEST"'" | .image.user_specified_image = "'"$REPO_TAG_DIGEST"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	# Testing for container start failed not because of the signature, but of
	# the missing command executable
	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"
	[[ "$output" == *"unable to start container process"* || "$output" == *"No such file or directory"* ]]
}

@test "deny unsigned image with restrictive policy on container creation7 if already pulled (by tag and ID)" {
	start_crio
	crictl_pull "$UNSIGNED_IMAGE"
	# Insert "latest" tag into the repoDigests field, and use that as the reference
	# CRI-O should filter out the :latest bit, so it's a valid reference for c/image
	REPO_TAG_DIGEST=$(crictl inspecti "$UNSIGNED_IMAGE" | jq -r .status.repoDigests[0] | sed "s|@|:latest@|g")
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$REPO_TAG_DIGEST"'" | .image.user_specified_image = "'"$REPO_TAG_DIGEST"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"

	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "deny signed image with restrictive policy on container creation if invalid policy (subjectEmail)" {
	start_crio
	crictl_pull "$SIGNED_IMAGE"
	# Insert "latest" tag into the repoDigests field, and use that as the reference
	# CRI-O should filter out the :latest bit, so it's a valid reference for c/image
	REPO_TAG_DIGEST=$(crictl inspecti "$SIGNED_IMAGE" | jq -r .status.repoDigests[0] | sed "s|@|:latest@|g")
	stop_crio_no_clean

	POLICY="$TESTDIR/policy.json"
	jq '.transports.docker["'"$SIGNED_IMAGE"'"][0].fulcio.subjectEmail = "invalid"' "$RESTRICTIVE_POLICY" > "$POLICY"
	SIGNATURE_POLICY="$POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$REPO_TAG_DIGEST"'" | .image.user_specified_image = "'"$REPO_TAG_DIGEST"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	# Testing for container start failed not because of the signature, but of
	# the missing command executable
	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"
	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "deny signed image with restrictive policy on container creation if invalid policy (oidcIssuer)" {
	start_crio
	crictl_pull "$SIGNED_IMAGE"
	# Insert "latest" tag into the repoDigests field, and use that as the reference
	# CRI-O should filter out the :latest bit, so it's a valid reference for c/image
	REPO_TAG_DIGEST=$(crictl inspecti "$SIGNED_IMAGE" | jq -r .status.repoDigests[0] | sed "s|@|:latest@|g")
	stop_crio_no_clean

	POLICY="$TESTDIR/policy.json"
	jq '.transports.docker["'"$SIGNED_IMAGE"'"][0].fulcio.oidcIssuer = "invalid"' "$RESTRICTIVE_POLICY" > "$POLICY"
	SIGNATURE_POLICY="$POLICY" start_crio
	POD_ID=$(crictl runp "$TESTDATA/sandbox_config.json")
	CTR_CONFIG="$TESTDIR/config.json"
	jq '.image.image = "'"$REPO_TAG_DIGEST"'" | .image.user_specified_image = "'"$REPO_TAG_DIGEST"'"' "$TESTDATA/container_config.json" > "$CTR_CONFIG"

	# Testing for container start failed not because of the signature, but of
	# the missing command executable
	run ! crictl create "$POD_ID" "$CTR_CONFIG" "$TESTDATA/sandbox_config.json"
	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "allow signed image mount" {
	if [[ "$TEST_USERNS" == "1" ]]; then
		skip "test fails in a user namespace"
	fi
	start_crio
	crictl_pull "$SIGNED_IMAGE"
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	jq --arg CONTAINER_PATH "$CONTAINER_PATH" --arg SIGNED_IMAGE "$SIGNED_IMAGE" \
		'.mounts = [{
			host_path: "",
			container_path: $CONTAINER_PATH,
			image: { image: $SIGNED_IMAGE, user_specified_image: $SIGNED_IMAGE },
			readonly: true
		}]' "$TESTDATA/container_config.json" > "$TESTDIR/container_config.json"

	crictl run "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json"
}

@test "deny unsigned image mount" {
	if [[ "$TEST_USERNS" == "1" ]]; then
		skip "test fails in a user namespace"
	fi
	start_crio
	crictl_pull "$UNSIGNED_IMAGE"
	stop_crio_no_clean

	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio
	jq --arg CONTAINER_PATH "$CONTAINER_PATH" --arg UNSIGNED_IMAGE "$UNSIGNED_IMAGE" \
		'.mounts = [{
			host_path: "",
			container_path: $CONTAINER_PATH,
			image: { image: $UNSIGNED_IMAGE, user_specified_image: $UNSIGNED_IMAGE },
			readonly: true
		}]' "$TESTDATA/container_config.json" > "$TESTDIR/container_config.json"

	run ! crictl run "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json"
	[[ "$output" == *"SignatureValidationFailed"* ]]
}

@test "accept ML-DSA-44 signed image over ML-DSA-44 TLS with ML-KEM" {
	setup_mldsa_image ML-DSA-44
	sign_in_registry "$MLDSA_IMAGE" "$TESTDIR/signing.key"
	start_crio_with_mldsa_policy "$TESTDIR/signing.pub"

	pull_mldsa_image_over_pqc_tls ML-DSA-44

	assert_log "$MLDSA_POLICY"
}

@test "accept ML-DSA-65 signed image over ML-DSA-65 TLS with ML-KEM" {
	setup_mldsa_image ML-DSA-65
	sign_in_registry "$MLDSA_IMAGE" "$TESTDIR/signing.key"
	start_crio_with_mldsa_policy "$TESTDIR/signing.pub"

	pull_mldsa_image_over_pqc_tls ML-DSA-65

	assert_log "$MLDSA_POLICY"
}

@test "accept ML-DSA-87 signed image over ML-DSA-87 TLS with ML-KEM" {
	setup_mldsa_image ML-DSA-87
	sign_in_registry "$MLDSA_IMAGE" "$TESTDIR/signing.key"
	start_crio_with_mldsa_policy "$TESTDIR/signing.pub"

	pull_mldsa_image_over_pqc_tls ML-DSA-87

	assert_log "$MLDSA_POLICY"
}

@test "deny ML-DSA signed image with untrusted key" {
	setup_mldsa_image ML-DSA-65
	sign_in_registry "$MLDSA_IMAGE" "$TESTDIR/signing.key"
	"$REGISTRY_BINARY" generate-key --algorithm ML-DSA-65 --prefix "$TESTDIR/untrusted"
	start_crio_with_mldsa_policy "$TESTDIR/untrusted.pub"

	run ! crictl pull "$REGISTRY_ADDRESS/$MLDSA_IMAGE"

	[[ "$output" == *"SignatureValidationFailed"* ]]
	[[ "$output" == *"cryptographic signature verification failed"* ]]
	assert_log "$MLDSA_POLICY"
}

@test "deny ML-DSA signed image with key of different parameter set" {
	setup_mldsa_image ML-DSA-65
	sign_in_registry "$MLDSA_IMAGE" "$TESTDIR/signing.key"
	"$REGISTRY_BINARY" generate-key --algorithm ML-DSA-44 --prefix "$TESTDIR/mldsa44"
	start_crio_with_mldsa_policy "$TESTDIR/mldsa44.pub"

	run ! crictl pull "$REGISTRY_ADDRESS/$MLDSA_IMAGE"

	[[ "$output" == *"SignatureValidationFailed"* ]]
	[[ "$output" == *"cryptographic signature verification failed"* ]]
	assert_log "$MLDSA_POLICY"
}

@test "deny unsigned image with ML-DSA policy" {
	setup_mldsa_image ML-DSA-65
	start_crio_with_mldsa_policy "$TESTDIR/signing.pub"

	run ! crictl pull "$REGISTRY_ADDRESS/$MLDSA_IMAGE"

	[[ "$output" == *"SignatureValidationFailed"* ]]
	assert_log "$MLDSA_POLICY"
}
