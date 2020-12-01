#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

@test "config migrate should succeed with default config" {
	output=$(crio -c "" -d "" config -m 1.17 2>&1)
	[[ "$output" != *"Changing"* ]]
}

@test "config migrate should succeed with 1.17 config" {
	# when
	output=$(crio -c "$TESTDATA/config/config-v1.17.0.toml" -d "" config -m 1.17 2>&1)

	# then
	[[ "$output" == *'Changing \"apparmor_profile\" to \"crio-default\"'* ]]
	[[ "$output" == *'apparmor_profile = "crio-default"'* ]]

	[[ "$output" == *'Removing \"default_capabilities\" entry \"NET_RAW\"'* ]]
	[[ "$output" == *'Removing \"default_capabilities\" entry \"SYS_CHROOT\"'* ]]

	[[ "$output" == *'Changing \"log_level\" to \"info\"'* ]]
	[[ "$output" == *'log_level = "info"'* ]]

	[[ "$output" == *'Changing \"ctr_stop_timeout\" to 30'* ]]
	[[ "$output" == *'ctr_stop_timeout = 30'* ]]
}

@test "config migrate should fail on invalid version" {
	# when
	run crio -c "" -d "" config -m 1.16

	# then
	[[ "$output" == *"unsupported migration version"* ]]
	[ "$status" -eq 1 ]
}
