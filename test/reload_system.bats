#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
	TEST_IMAGE=quay.io/saschagrunert/hello-world
	TEST_IMAGE2=quay.io/crio/hello-wasm
	export CONTAINER_REGISTRIES_CONF="$TESTDIR/containers/registries.conf"
	export CONTAINER_REGISTRIES_CONF_DIR="$TESTDIR/containers/registries.conf.d"
	mkdir -p "$CONTAINER_REGISTRIES_CONF_DIR"
	printf "[[registry]]\nlocation = 'quay.io/saschagrunert'\nblocked = true" \
		>> "$CONTAINER_REGISTRIES_CONF"
	printf "[[registry]]\nlocation = 'quay.io/crio'\nblocked = true" \
		>> "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"
}

function drop_in_for_auto_reload_registries() {
	cat << EOF > "$CRIO_CONFIG_DIR/00-auto-reload-registries.conf"
[crio.image]
auto_reload_registries = true
[crio.runtime]
log_level = "debug"
EOF
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

function count_log_entries() {
	local search_pattern="$1"
	local count

	count=$(grep -c "$search_pattern" "$CRIO_LOG")

	echo "$count"
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
	drop_in_for_auto_reload_registries
	start_crio

	# when
	sed -i 's;true;false;g' "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"

	# then
	expect_log_success
	crictl pull "$TEST_IMAGE2"
}

@test "system registries should fail on invalid syntax in file without reload" {
	# given
	drop_in_for_auto_reload_registries
	start_crio
	echo invalid >> "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"

	# then
	expect_log_failure
}

@test "system handles burst of configuration changes without excessive reloads" {
	# given
	drop_in_for_auto_reload_registries
	start_crio

	sed -i 's;true;false;g' "$CONTAINER_REGISTRIES_CONF"
	sed -i 's;true;false;g' "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"
	cat << EOF >> "$CONTAINER_REGISTRIES_CONF_DIR/02-registry.conf"
[[registry]]
location = 'registry.k8s.io/pause'
blocked = false
EOF
	sleep 1
	# then
	log_count=$(count_log_entries "Applied new registry configuration")
	[ "$log_count" -eq 1 ]
}

@test "system handles duplicate events for the same file" {
	# given
	drop_in_for_auto_reload_registries
	start_crio

	# when
	for _ in {1..5}; do
		sed -i 's;true;false;g' "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"
	done
	sleep 1
	# then
	log_count=$(count_log_entries "Applied new registry configuration")
	[ "$log_count" -eq 1 ]
}
