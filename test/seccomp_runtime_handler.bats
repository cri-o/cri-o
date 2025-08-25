#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	if ! "$CHECKSECCOMP_BINARY"; then
		skip "seccomp is not enabled"
	fi

	setup_test
}

function teardown() {
	cleanup_test
}

TEST_SYSCALL_HANDLER=RUNTIME_HANDLER_PROFILE_APPLIED
TEST_SYSCALL_RUNTIME=RUNTIME_CONFIG_PROFILE_APPLIED

@test "runtime handler seccomp profile takes precedence over runtime config seccomp profile" {
	setup_crio

	jq --arg SYSCALL "$TEST_SYSCALL_HANDLER" \
		'.syscalls[0].names += [$SYSCALL]' \
		"$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/profile.json

	create_runtime_with_seccomp_profile seccomp "$CONTAINER_SECCOMP_PROFILE" "$TESTDIR"/profile.json
	start_crio_no_setup

	# profile_type = RuntimeDefault
	jq '.linux.security_context.seccomp.profile_type = 0' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container.json

	CTR=$(crictl run "$TESTDIR"/container.json "$TESTDATA"/sandbox_config.json)

	grep -q "Applied runtime handler seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL_HANDLER
}

@test "when runtime handler seccomp profile is not set, falls back to runtime config seccomp profile" {
	setup_crio

	jq --arg SYSCALL "$TEST_SYSCALL_RUNTIME" \
		'.syscalls[0].names += [$SYSCALL]' \
		"$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/profile.json

	create_runtime_with_seccomp_profile seccomp "$TESTDIR"/profile.json ""
	start_crio_no_setup

	# profile_type = RuntimeDefault
	jq '.linux.security_context.seccomp.profile_type = 0' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container.json

	CTR=$(crictl run "$TESTDIR"/container.json "$TESTDATA"/sandbox_config.json)

	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL_RUNTIME
}

@test "runtime handler seccomp profile should not be applied when profile is not RuntimeDefault" {
	setup_crio

	jq --arg SYSCALL "$TEST_SYSCALL_HANDLER" \
		'.syscalls[0].names += [$SYSCALL]' \
		"$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/profile.json

	create_runtime_with_seccomp_profile seccomp "$CONTAINER_SECCOMP_PROFILE" "$TESTDIR"/profile.json
	start_crio_no_setup

	# profile_type = Unconfined
	jq '.linux.security_context.seccomp.profile_type = 1' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container.json

	CTR=$(crictl run "$TESTDIR"/container.json "$TESTDATA/sandbox_config.json")

	crictl inspect "$CTR" | jq -e '.info.runtimeSpec.linux.seccomp == null'
}

@test "runtime handler seccomp profile file does not exist" {
	setup_crio

	create_runtime_with_seccomp_profile seccomp "" /not-exist
	run ! start_crio_no_setup

	grep -q "stat /not-exist: no such file or directory" "$CRIO_LOG"
}

@test "runtime handler seccomp profile file is not an absolute path" {
	setup_crio

	create_runtime_with_seccomp_profile seccomp "" profile.json
	run ! start_crio_no_setup

	grep -q "seccomp_profile for runtime 'seccomp' is not absolute: profile.json" "$CRIO_LOG"
}

@test "runtime handler seccomp profile file is invalid" {
	setup_crio

	echo "{{}}" > "$TESTDIR"/profile.json
	create_runtime_with_seccomp_profile seccomp "" "$TESTDIR"/profile.json
	start_crio_no_setup

	# profile_type = RuntimeDefault
	jq '.linux.security_context.seccomp.profile_type = 0' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container.json

	run ! crictl run "$TESTDIR"/container.json "$TESTDATA"/sandbox_config.json

	grep -q "decoding seccomp profile failed: invalid character" "$CRIO_LOG"
}
