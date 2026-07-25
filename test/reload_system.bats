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

function assert_debounced_reloads() {
	# A burst normally coalesces into one reload, but a slow machine may fire
	# the debounce timer more than once; consecutive reloads must still be at
	# least the debounce interval apart (50ms slack for timestamp lag). We use
	# the "reloading" log line (timer firing) rather than "Applied" (reload
	# done) to measure timer cadence; success is checked by expect_log_success.
	local -a reload_times
	readarray -t reload_times < <(grep "reloading registries configuration" "$CRIO_LOG" |
		grep -oE 'time="[^"]*"' | cut -d'"' -f2)

	[ "${#reload_times[@]}" -ge 1 ]

	if [ "${#reload_times[@]}" -gt 1 ]; then
		local timestamp prev_ns=0 curr_ns
		for timestamp in "${reload_times[@]}"; do
			if ! curr_ns=$(date -d "$timestamp" +%s%N); then
				echo "could not parse reload timestamp: $timestamp" >&2
				return 1
			fi
			if [ "$prev_ns" -gt 0 ]; then
				[ $((curr_ns - prev_ns)) -ge 150000000 ]
			fi
			prev_ns=$curr_ns
		done
	fi
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

	# when
	sed -i 's;true;false;g' "$CONTAINER_REGISTRIES_CONF"
	sed -i 's;true;false;g' "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"
	cat << EOF >> "$CONTAINER_REGISTRIES_CONF_DIR/02-registry.conf"
[[registry]]
location = 'registry.k8s.io/pause'
blocked = false
EOF

	# then
	expect_log_success
	sleep 1
	assert_debounced_reloads
}

@test "system handles duplicate events for the same file" {
	# given
	drop_in_for_auto_reload_registries
	start_crio

	# when
	for _ in {1..5}; do
		sed -i 's;true;false;g' "$CONTAINER_REGISTRIES_CONF_DIR/01-registry.conf"
	done

	# then
	expect_log_success
	sleep 1
	assert_debounced_reloads
}
