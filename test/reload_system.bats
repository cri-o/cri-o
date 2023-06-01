#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
	TEST_IMAGE=quay.io/saschagrunert/hello-world
	export CONTAINER_REGISTRIES_CONF="$TESTDIR/containers/registries.conf"
	mkdir "$TESTDIR/containers"
	printf "[[registry]]\nlocation = 'quay.io/saschagrunert'\nblocked = true" \
		>> "$CONTAINER_REGISTRIES_CONF"
	start_crio
}

function teardown() {
	cleanup_test
}

function expect_log_success() {
	wait_for_log "applied new registry configuration"
}

function expect_log_failure() {
	wait_for_log "system registries reload failed"
}

@test "reload system registries should succeed" {
	# given
	replace_config "log_level" "debug"

	# when
	reload_crio

	# then
	expect_log_success
	run ! crictl pull "$TEST_IMAGE"
}

@test "reload system registries should succeed with new registry" {
	# given
	replace_config "log_level" "debug"
	sed -i 's;true;false;g' "$CONTAINER_REGISTRIES_CONF"

	# when
	reload_crio

	# then
	expect_log_success
	crictl pull "$TEST_IMAGE"
}

@test "reload system registries should fail on invalid syntax in file" {
	# given
	echo invalid >> "$CONTAINER_REGISTRIES_CONF"

	# when
	reload_crio

	# then
	expect_log_failure
}
