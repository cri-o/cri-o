#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "config dir should succeed" {
	# given
	setup_crio

	printf "[crio.runtime]\npids_limit = 1234\n" > "$CRIO_CONFIG_DIR"/00-default
	printf "[crio.runtime]\npids_limit = 5678\n" > "$CRIO_CONFIG_DIR"/01-overwrite

	# when
	start_crio_no_setup
	output=$("${CRIO_STATUS_BINARY_PATH}" --socket="${CRIO_SOCKET}" config)

	# then
	[[ "$output" == *"pids_limit = 5678"* ]]
}

@test "config dir should fail with invalid option" {
	# given
	printf '[crio.runtime]\nlog_level = "info"\n' > "$CRIO_CONFIG"
	printf '[crio.runtime]\nlog_level = "wrong-level"\n' > "$CRIO_CONFIG_DIR"/00-default

	# when
	run "$CRIO_BINARY_PATH" -c "$CRIO_CONFIG" -d "$CRIO_CONFIG_DIR"

	# then
	[ "$status" -ne 0 ]
	[[ "$output" == *"not a valid logrus"*"wrong-level"* ]]
}

@test "replace default runtime should succeed" {
	# when
	RES=$("$CRIO_BINARY_PATH" -d "$TESTDATA"/50-crun-default.conf config 2>&1)

	# then
	[[ "$RES" == *"default_runtime = \"crun\""* ]]
	[[ "$RES" != *"crio.runtime.runtimes.runc"* ]]
	[[ "$RES" == *"crio.runtime.runtimes.crun"* ]]
}

@test "retain default runtime should succeed" {
	# when
	RES=$("$CRIO_BINARY_PATH" -d "$TESTDATA"/50-crun.conf config 2>&1)

	# then
	[[ "$RES" == *"default_runtime = \"runc\""* ]]
	[[ "$RES" == *"crio.runtime.runtimes.runc"* ]]
	[[ "$RES" == *"crio.runtime.runtimes.crun"* ]]
}
