#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function start_crio_without_debug() {
	"$CRIO_BINARY_PATH" \
		--default-mounts-file "$TESTDIR/containers/mounts.conf" \
		-c "$CRIO_CONFIG" \
		-d "$CRIO_CONFIG_DIR" \
		&> "$CRIO_LOG" &
	CRIO_PID=$!
	wait_until_reachable
}

# Start crio.
# shellcheck disable=SC2120
function start_crio_no_log_level() {
	setup_crio "$@"
	start_crio_without_debug
	check_images
}

function reload_crio() {
	kill -HUP "$CRIO_PID"
}

function setup() {
	setup_test
	start_crio_no_log_level

	# the default log_level is `error` so we have to adapt it before running
	# any tests to be able to see the `info` messages
	replace_config "log_level" "debug"
}

function teardown() {
	rm -f "$CRIO_CONFIG_DIR/00-new*Runtime.conf"
	cleanup_test
}

function expect_log_success() {
	wait_for_log '"set config '"$1"' to \\"'"$2"'\\""'
}

function expect_log_failure() {
	wait_for_log "unable to reload configuration: $1"
}

@test "reload config should succeed with 'log_level'" {
	# given
	NEW_LEVEL="warn"
	OPTION="log_level"

	# when
	replace_config $OPTION $NEW_LEVEL
	reload_crio

	# then
	expect_log_success $OPTION $NEW_LEVEL
}

@test "reload config should fail with 'log_level' if invalid" {
	# when
	replace_config "log_level" "invalid"
	reload_crio

	# then
	expect_log_failure "not a valid logrus Level"
}
