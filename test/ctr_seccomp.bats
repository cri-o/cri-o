#!/usr/bin/env bats

load helpers

function setup() {
	if ! "$CHECKSECCOMP_BINARY"; then
		skip "seccomp is not enabled"
	fi

	setup_test

	sed -e 's/"chmod",//' -e 's/"fchmod",//' -e 's/"fchmodat",//g' \
		"$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json

	CONTAINER_SECCOMP_PROFILE="$TESTDIR"/seccomp_profile1.json start_crio
}

function teardown() {
	cleanup_test
}

# 1. test running with ctr unconfined
# test that we can run with a syscall which would be otherwise blocked
@test "ctr seccomp profiles unconfined" {
	jq '	  .linux.security_context.seccomp_profile_path = "unconfined"' \
		"$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/seccomp1.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl exec --sync "$ctr_id" chmod 777 .
}

# 2. test running with ctr runtime/default
# test that we cannot run with a syscall blocked by the default seccomp profile
@test "ctr seccomp profiles runtime/default" {
	jq '	  .linux.security_context.seccomp_profile_path = "runtime/default"' \
		"$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp2.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/seccomp2.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	run crictl exec --sync "$ctr_id" chmod 777 .
	[ "$status" -ne 0 ]
	[[ "$output" == *"Operation not permitted"* ]]
}

# 3. test running with ctr unconfined and profile empty
# test that we can run with a syscall which would be otherwise blocked
@test "ctr seccomp profiles unconfined by empty field" {
	jq '	  .linux.security_context.seccomp_profile_path = ""' \
		"$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/seccomp1.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl exec --sync "$ctr_id" chmod 777 .
}

# 4. test running with ctr wrong profile name
@test "ctr seccomp profiles wrong profile name" {
	jq '	  .linux.security_context.seccomp_profile_path = "wontwork"' \
		"$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	run crictl create "$pod_id" "$TESTDIR"/seccomp1.json "$TESTDATA"/sandbox_config.json
	[[ "$status" -ne 0 ]]
	[[ "$output" =~ "unknown seccomp profile option:" ]]
	[[ "$output" =~ "wontwork" ]]
}

# 5. test running with ctr localhost/profile_name
@test "ctr seccomp profiles localhost/profile_name" {
	jq '	  .linux.security_context.seccomp_profile_path = "localhost/'"$TESTDIR"'/seccomp_profile1.json"' \
		"$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/seccomp1.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	run crictl exec --sync "$ctr_id" chmod 777 .
	[ "$status" -ne 0 ]
	[[ "$output" == *"Operation not permitted"* ]]
}

# 6. test running with ctr docker/default
# test that we cannot run with a syscall blocked by the default seccomp profile
@test "ctr seccomp profiles docker/default" {
	jq '	  .linux.security_context.seccomp_profile_path = "docker/default"' \
		"$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp2.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/seccomp2.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	run crictl exec --sync "$ctr_id" chmod 777 .
	[ "$status" -ne 0 ]
	[[ "$output" == *"Operation not permitted"* ]]
}

# 7. test running with ctr runtime/default if seccomp_override_empty is true
# test that we cannot run with a syscall blocked by the default seccomp profile
@test "ctr seccomp overrides unconfined profile with runtime/default when overridden" {
	export CONTAINER_SECCOMP_USE_DEFAULT_WHEN_EMPTY=true
	export CONTAINER_SECCOMP_PROFILE="$TESTDIR"/seccomp_profile1.json
	restart_crio

	jq '	  .linux.security_context.seccomp_profile_path = ""' \
		"$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/seccomp1.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	run crictl exec --sync "$ctr_id" chmod 777 .
	[ "$status" -ne 0 ]
	[[ "$output" == *"Operation not permitted"* ]]
}
