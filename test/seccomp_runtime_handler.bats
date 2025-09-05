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

@test "seccomp runtime handler profile takes precedence over runtime config" {
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
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL_HANDLER
}

@test "seccomp runtime handler profile falls back to runtime config when not set" {
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

@test "seccomp runtime handler profile should not apply when profile is not RuntimeDefault" {
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

@test "seccomp runtime handler profile loads for pod" {
	setup_crio

	jq --arg SYSCALL "$TEST_SYSCALL_HANDLER" \
		'.syscalls[0].names += [$SYSCALL]' \
		"$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/profile.json

	create_runtime_with_seccomp_profile seccomp "$CONTAINER_SECCOMP_PROFILE" "$TESTDIR"/profile.json
	start_crio_no_setup

	# profile_type = RuntimeDefault
	jq '.linux.security_context.seccomp.profile_type = 0' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	POD=$(crictl runp "$TESTDIR"/sandbox.json)
	crictl inspectp "$POD" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL_HANDLER
}

@test "seccomp runtime handler profile not exists" {
	setup_crio

	create_runtime_with_seccomp_profile seccomp "" "/not-exist"
	run ! start_crio_no_setup

	grep -q "open /not-exist: no such file or directory" "$CRIO_LOG"
}

@test "seccomp runtime handler profile is invalid" {
	setup_crio

	echo "{{}}" > "$TESTDIR"/profile.json
	create_runtime_with_seccomp_profile seccomp "" "$TESTDIR"/profile.json
	run ! start_crio_no_setup

	grep -q "decoding seccomp profile failed" "$CRIO_LOG"
}
