#!/usr/bin/env bats
# Integration tests for the pinned_artifacts pre-pull feature.
#
# Tests cover:
#   - CLI: "crio artifact pull" argument validation and successful pull
#   - Config: pinned_artifacts field acceptance
#   - Server startup: background pre-pull logging behavior
#   - Config reload (SIGHUP): updated list triggers pull; identical list is a no-op

load helpers

ARTIFACT_REPO="quay.io/crio/artifact"
ARTIFACT_IMAGE="$ARTIFACT_REPO:singlefile"
ARTIFACT_IMAGE_2="$ARTIFACT_REPO:multiplefiles"

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

# ---------------------------------------------------------------------------
# CLI: "crio artifact pull" — argument validation (no running server needed)
# ---------------------------------------------------------------------------

@test "crio artifact pull: missing argument exits non-zero with usage message" {
	run -1 "$CRIO_BINARY_PATH" -c /dev/null artifact pull
	[[ "$output" == *"usage: crio artifact pull"* ]]
}

@test "crio artifact pull: extra argument exits non-zero with usage message" {
	run -1 "$CRIO_BINARY_PATH" -c /dev/null artifact pull ref1 extra
	[[ "$output" == *"usage: crio artifact pull"* ]]
}

# ---------------------------------------------------------------------------
# Config: pinned_artifacts field is accepted in the config
# ---------------------------------------------------------------------------

@test "pinned_artifacts config field is accepted and appears in status output" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = ["$ARTIFACT_IMAGE"]
EOF
	start_crio
	run -0 "$CRIO_BINARY_PATH" status --socket="$CRIO_SOCKET" config
	[[ "$output" == *"pinned_artifacts"* ]]
}

@test "empty pinned_artifacts config is accepted without error" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = []
EOF
	start_crio
	run -0 "$CRIO_BINARY_PATH" status --socket="$CRIO_SOCKET" config
	[[ "$output" == *"pinned_artifacts"* ]]
}

# ---------------------------------------------------------------------------
# Server startup: pre-pull log behavior (start_crio_no_setup already passes -l debug)
# ---------------------------------------------------------------------------

@test "startup with one pinned artifact logs pre-pull count" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = ["$ARTIFACT_IMAGE"]
EOF
	start_crio
	wait_for_log "Pre-pulling 1 pinned artifact"
}

@test "startup with two pinned artifacts logs correct count" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = [
	"$ARTIFACT_IMAGE",
	"$ARTIFACT_IMAGE_2"
]
EOF
	start_crio
	wait_for_log "Pre-pulling 2 pinned artifact"
}

@test "startup with empty pinned_artifacts does not log a pre-pull message" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = []
EOF
	start_crio
	run ! grep -i "pre-pulling.*pinned" "$CRIO_LOG"
}

@test "unparsable pinned artifact ref is logged as error and server still starts" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = ["://not-a-valid-ref"]
EOF
	start_crio
	wait_for_log "Failed to parse pinned artifact reference"
	# Server must still be reachable despite the bad ref
	crictl info
}

@test "bad pinned artifact ref does not block startup of subsequent refs" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = [
	"://bad-ref",
	"$ARTIFACT_IMAGE"
]
EOF
	start_crio
	# The bad ref is skipped, the good one is attempted
	wait_for_log "Pre-pulling 2 pinned artifact"
	wait_for_log "Failed to parse pinned artifact reference"
}

# ---------------------------------------------------------------------------
# Config reload: SIGHUP applies updated pinned_artifacts list
# ---------------------------------------------------------------------------

@test "reload config with new pinned_artifacts logs config-update entry" {
	setup_crio
	# Persist debug log level so it survives the SIGHUP reload.
	replace_config "log_level" "debug"
	start_crio_no_setup

	printf '[crio.image]\npinned_artifacts = ["%s"]\n' "$ARTIFACT_IMAGE" \
		> "$CRIO_CONFIG_DIR/99-pinned.conf"
	reload_crio

	wait_for_log "set config pinned_artifacts to"
}

@test "reload config with new pinned_artifacts triggers pre-pull" {
	setup_crio
	replace_config "log_level" "debug"
	start_crio_no_setup

	# Server started with empty pinned_artifacts; no pre-pull should have fired.
	run ! grep -i "pre-pulling.*pinned" "$CRIO_LOG"

	printf '[crio.image]\npinned_artifacts = ["%s"]\n' "$ARTIFACT_IMAGE" \
		> "$CRIO_CONFIG_DIR/99-pinned.conf"
	reload_crio

	wait_for_log "Pre-pulling 1 pinned artifact"
}

@test "reload config with empty pinned_artifacts logs cleared list" {
	setup_crio
	replace_config "log_level" "debug"
	printf '[crio.image]\npinned_artifacts = ["%s"]\n' "$ARTIFACT_IMAGE" \
		> "$CRIO_CONFIG_DIR/99-pinned.conf"
	start_crio_no_setup
	wait_for_log "Pre-pulling 1 pinned artifact"

	printf '[crio.image]\npinned_artifacts = []\n' \
		> "$CRIO_CONFIG_DIR/99-pinned.conf"
	reload_crio

	wait_for_log 'set config pinned_artifacts to.*\[\]'
}

@test "reload config with identical pinned_artifacts does not log a config-update" {
	setup_crio
	replace_config "log_level" "debug"
	printf '[crio.image]\npinned_artifacts = ["%s"]\n' "$ARTIFACT_IMAGE" \
		> "$CRIO_CONFIG_DIR/99-pinned.conf"
	start_crio_no_setup

	reload_crio
	wait_for_log "Configuration reload completed"

	# Identical lists must not trigger a logConfig("pinned_artifacts", ...) call.
	run ! grep -i "set config pinned_artifacts" "$CRIO_LOG"
}

@test "reload config with order-swapped pinned_artifacts does not log a config-update" {
	setup_crio
	replace_config "log_level" "debug"
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = ["$ARTIFACT_IMAGE", "$ARTIFACT_IMAGE_2"]
EOF
	start_crio_no_setup

	# Reverse the order — sorted comparison should treat this as identical.
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = ["$ARTIFACT_IMAGE_2", "$ARTIFACT_IMAGE"]
EOF
	reload_crio
	wait_for_log "Configuration reload completed"

	run ! grep -i "set config pinned_artifacts" "$CRIO_LOG"
}

# ---------------------------------------------------------------------------
# Network: actual artifact pull (requires registry access)
# ---------------------------------------------------------------------------

@test "pinned_artifacts pre-pulls artifact at startup and logs success" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = ["$ARTIFACT_IMAGE"]
EOF
	start_crio
	wait_for_log "Pinned artifact pre-pulled: $ARTIFACT_IMAGE"
}

@test "crio artifact pull: CLI pulls artifact into store and prints success" {
	# Initialize container storage via a normal crio start.
	start_crio

	run -0 "$CRIO_BINARY_PATH" \
		-c "$CRIO_CONFIG" \
		-d "$CRIO_CONFIG_DIR" \
		artifact pull "$ARTIFACT_IMAGE"
	[[ "$output" == *"Successfully pulled artifact"* ]]
}

@test "pinned_artifacts pull failure on bad network ref does not crash server" {
	cat << EOF > "$CRIO_CONFIG_DIR/99-pinned.conf"
[crio.image]
pinned_artifacts = ["localhost:65535/nonexistent:latest"]
EOF
	start_crio
	wait_for_log "Pre-pulling 1 pinned artifact"
	wait_for_log "Failed to pull pinned artifact"

	# Server must remain reachable.
	crictl info
}
