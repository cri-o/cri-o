#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
	start_crio

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

@test "reload config should succeed" {
	# when
	reload_crio

	# then
	ps --pid "$CRIO_PID" &> /dev/null
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

@test "reload config should fail with if config is malformed" {
	# when
	replace_config "log_level" '\"'
	reload_crio

	# then
	expect_log_failure "unable to decode configuration"
}

@test "reload config should succeed with 'pause_image'" {
	# given
	NEW_OPTION="new-image"
	OPTION="pause_image"

	# when
	replace_config $OPTION $NEW_OPTION
	reload_crio

	# then
	expect_log_success $OPTION $NEW_OPTION
}

@test "reload config should succeed with 'pause_command'" {
	# given
	NEW_OPTION="new-command"
	OPTION="pause_command"

	# when
	replace_config $OPTION $NEW_OPTION
	reload_crio

	# then
	expect_log_success $OPTION $NEW_OPTION
}

@test "reload config should succeed with 'pause_image_auth_file'" {
	# given
	NEW_OPTION="$TESTDIR/auth_file"
	OPTION="pause_image_auth_file"
	touch "$NEW_OPTION"

	# when
	replace_config $OPTION "$NEW_OPTION"
	reload_crio

	# then
	expect_log_success $OPTION "$NEW_OPTION"
}

@test "reload config should fail with non existing 'pause_image_auth_file'" {
	# given
	NEW_OPTION="$TESTDIR/auth_file"
	OPTION="pause_image_auth_file"

	# when
	replace_config $OPTION "$NEW_OPTION"
	reload_crio

	# then
	expect_log_failure "stat $NEW_OPTION"
}

@test "reload config should succeed with 'log_filter'" {
	# given
	NEW_FILTER="new"
	OPTION="log_filter"

	# when
	replace_config $OPTION $NEW_FILTER
	reload_crio

	# then
	expect_log_success $OPTION $NEW_FILTER
}

@test "reload config should fail with invalid 'log_filter'" {
	# given
	NEW_FILTER=")"
	OPTION="log_filter"

	# when
	replace_config $OPTION $NEW_FILTER
	reload_crio

	# then
	expect_log_failure "custom log level filter does not compile"
}

@test "reload config should succeed with 'decryption_keys_path'" {
	# given
	NEW_OPTION="/etc/crio"
	OPTION="decryption_keys_path"

	# when
	replace_config $OPTION $NEW_OPTION
	reload_crio

	# then
	expect_log_success $OPTION $NEW_OPTION
}

@test "reload config should succeed with 'seccomp_profile'" {
	# given
	NEW_SECCOMP_PROFILE="$(mktemp --tmpdir seccomp.XXXXXX.json)"
	echo "{}" > "$NEW_SECCOMP_PROFILE"
	OPTION="seccomp_profile"

	# when
	replace_config $OPTION "$NEW_SECCOMP_PROFILE"
	reload_crio

	# then
	expect_log_success $OPTION "$NEW_SECCOMP_PROFILE"
}

@test "reload config should not fail with invalid 'seccomp_profile'" {
	# given
	NEW_SECCOMP_PROFILE=")"
	OPTION="seccomp_profile"

	# when
	replace_config $OPTION $NEW_SECCOMP_PROFILE
	reload_crio

	# then
	wait_for_log "Specified profile does not exist on disk"
}

@test "reload config should succeed with 'apparmor_profile'" {
	if ! is_apparmor_enabled; then
		skip "apparmor not enabled"
	fi

	# given
	NEW_APPARMOR_PROFILE="unconfined"
	OPTION="apparmor_profile"

	# when
	replace_config $OPTION $NEW_APPARMOR_PROFILE
	reload_crio

	# then
	expect_log_success $OPTION $NEW_APPARMOR_PROFILE
}

@test "reload config should fail with invalid 'apparmor_profile'" {
	if ! is_apparmor_enabled; then
		skip "apparmor not enabled"
	fi

	# given
	NEW_APPARMOR_PROFILE=")"
	OPTION="apparmor_profile"

	# when
	replace_config $OPTION $NEW_APPARMOR_PROFILE
	reload_crio

	# then
	expect_log_failure "unable to reload apparmor_profile"
}

@test "reload config should add new runtime" {
	# given
	cat << EOF > "$CRIO_CONFIG_DIR/00-newRuntime.conf"
[crio.runtime.runtimes.new]
runtime_path = "$RUNTIME_BINARY_PATH"
EOF

	# when
	reload_crio

	#then
	wait_for_log '"updating runtime configuration"'
}

@test "reload config should update 'pinned_images'" {
	OPTION="pinned_images"
	EXAMPLE_IMAGE=quay.io/crio/fedora-crio-ci:latest
	# add pinned_images config
	printf '[crio.image]\npinned_images = [""]\n' > "$CRIO_CONFIG_DIR"/00-default
	printf '[crio.image]\npinned_images = ["%s"]\n' $EXAMPLE_IMAGE > "$CRIO_CONFIG_DIR"/01-overwrite
	# image is not pinned
	output=$(crictl images -o json | jq ".images[] | select(.repoTags[] == \"$EXAMPLE_IMAGE\") |.pinned")
	[ "$output" == "false" ]
	reload_crio
	# image becomes pinned
	expect_log_success $OPTION $EXAMPLE_IMAGE
	wait_for_log "Configuration reload completed"
	output=$(crictl images -o json | jq ".images[] | select(.repoTags[] == \"$EXAMPLE_IMAGE\") |.pinned")
	[ "$output" == "true" ]
}

@test "reload config should update 'pinned_images' and only 'pause_image' is pinned" {
	OPTION="pause_image"
	printf '[crio.image]\npinned_images = [""]\n' > "$CRIO_CONFIG_DIR"/00-default
	reload_crio
	wait_for_log "Configuration reload completed"
	output=$(crictl images -o json | jq '.images[] | select(.pinned == true) | .repoTags[]')
	# only pause image is pinned
	[[ "$output" == *"pause"* ]]
}

@test "reload config should update 'pause_image' and it becomes 'pinned_images'" {
	OPTION="pause_image"
	EXAMPLE_IMAGE=quay.io/crio/fedora-crio-ci:latest
	printf '[crio.image]\npinned_images = [""]\npause_image = "%s"\n' $EXAMPLE_IMAGE > "$CRIO_CONFIG_DIR"/04-overwrite
	reload_crio
	expect_log_success $OPTION $EXAMPLE_IMAGE
	wait_for_log "Configuration reload completed"
	output=$(crictl images -o json | jq '.images[] | select(.pinned == true) | .repoTags[]')
	# pause image is pinned
	[[ "$output" == *"fedora-crio-ci"* ]]
}

@test "reload config should remove pinned images when an empty list is provided" {
	EXAMPLE_IMAGE=quay.io/crio/fedora-crio-ci:latest

	# Add a pinned image to the configuration
	printf '[crio.image]\npinned_images = ["%s", ""]\n' $EXAMPLE_IMAGE > "$CRIO_CONFIG_DIR"/01-overwrite
	reload_crio
	wait_for_log 'Set config pinned_images to \\"quay.io/crio/fedora-crio-ci:latest\\"'
	wait_for_log "Configuration reload completed"
	wait_for_log 'pinned_images = \[\\"quay.io/crio/fedora-crio-ci:latest\\"\]'

	# Verify that the image is pinned
	output=$(crictl images -o json | jq ".images[] | select(.repoTags[] == \"$EXAMPLE_IMAGE\") |.pinned")
	[ "$output" == "true" ]

	# Remove the pinned image from the configuration
	printf '[crio.image]\npinned_images = []\n' > "$CRIO_CONFIG_DIR"/01-overwrite
	reload_crio
	wait_for_log 'Set config pinned_images to \\"\[\]\\"'
	wait_for_log "Configuration reload completed"
	wait_for_log 'pinned_images = \[\]'

	# Verify that the image is no longer pinned
	output=$(crictl images -o json | jq ".images[] | select(.repoTags[] == \"$EXAMPLE_IMAGE\") |.pinned")
	[ "$output" == "false" ]
}
