#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

RESTRICTIVE_POLICY="$INTEGRATION_ROOT/policy-signature.json"

REGISTRY="quay.io/crio"
SIGNED_IMAGE="$REGISTRY/signed"

@test "pull signed image with restrictive policy discovers signatures via referrers" {
	SIGNATURE_POLICY="$RESTRICTIVE_POLICY" start_crio

	crictl_pull "$SIGNED_IMAGE"

	# Verify the referrers code path was exercised. The log will show
	# either the API response or the tag schema fallback.
	grep -qE "Looking for OCI referrers for|Looking for OCI referrers via tag schema" "$CRIO_LOG"
}

@test "pull signed image exercises referrers discovery" {
	start_crio

	crictl_pull "$SIGNED_IMAGE"

	# With use-sigstore-attachments enabled (test/default.yaml), the
	# referrers discovery code path must run during pull.
	grep -qE "Looking for OCI referrers for|Looking for OCI referrers via tag schema" "$CRIO_LOG"
	# Must not see "disabled by configuration" since the flag is on.
	run ! grep -q "Not looking for sigstore referrers: disabled by configuration" "$CRIO_LOG"
}

@test "pull unsigned image with referrers discovery does not fail" {
	start_crio

	crictl_pull "${IMAGES[1]}"

	# Referrers discovery should run without errors even for unsigned images.
	grep -qE "Looking for OCI referrers for|Looking for OCI referrers via tag schema" "$CRIO_LOG"
	# The pull must succeed (verified by crictl_pull not failing above).
}
