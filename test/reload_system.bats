#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
	TEST_IMAGE=quay.io/saschagrunert/hello-world
	TEST_IMAGE2=quay.io/crio/hello-wasm
	export CONTAINER_REGISTRIES_CONF="$TESTDIR/containers/registries.conf"
	export CONTAINER_REGISTRIES_CONF_DIR="$TESTDIR/containers/registries.conf.d"
	mkdir -p "$TESTDIR/containers/registries.conf.d"
	printf "[[registry]]\nlocation = 'quay.io/saschagrunert'\nblocked = true" \
		>> "$CONTAINER_REGISTRIES_CONF"
	printf "[[registry]]\nlocation = 'quay.io/crio'\nblocked = true" \
		>> "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"
}

function teardown() {
	cleanup_test
}

function expect_log_success() {
	wait_for_log "applied new registry configuration"
}

function expect_log_warning() {
	wait_for_log "rate limit exceeded, skipping reload"
}

function expect_log_failure() {
	wait_for_log "system registries reload failed"
}

@test "reload system registries should succeed" {
	# given
	start_crio
	replace_config "log_level" "debug"

	# when
	reload_crio

	# then
	expect_log_success
	run ! crictl pull "$TEST_IMAGE"
}

@test "reload system registries should succeed with new registry" {
	# given
	start_crio
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
	start_crio
	echo invalid >> "$CONTAINER_REGISTRIES_CONF"

	# when
	reload_crio

	# then
	expect_log_failure
}

@test "system registries should succeed with new registry without reload" {
	# given
	cat << EOF > "$CRIO_CONFIG_DIR/00-automaticReload.conf"
[crio.runtime]
automatic_reload_mirror_registry = true
EOF
	start_crio
	replace_config "log_level" "debug"

	# when
	sed -i 's;true;false;g' "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"

	# then
	expect_log_success
	crictl pull "$TEST_IMAGE2"
}

@test "system registries should fail on invalid syntax in file without reload" {
	# given
	cat << EOF > "$CRIO_CONFIG_DIR/00-automaticReload.conf"
[crio.runtime]
automatic_reload_mirror_registry = true
EOF
	start_crio
	echo invalid >> "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"

	# then
	expect_log_failure
}

@test "system registries should reach the rate limit for more than 10 requests per minute" {
	# given
	cat << EOF > "$CRIO_CONFIG_DIR/00-automaticReload.conf"
[crio.runtime]
automatic_reload_mirror_registry = true
EOF
	start_crio
	replace_config "log_level" "debug"

	# when
	for i in {1..11}; do
		touch "$CONTAINER_REGISTRIES_CONF_DIR/$i-registry.conf"
		sleep 1 # Adjust the sleep duration as needed
	done

	# then
	expect_log_warning
}
