#!/usr/bin/env bats

load helpers

# bats file_tags=serial

function setup() {
	if ! is_apparmor_enabled; then
		skip "apparmor not enabled"
	fi
	setup_test
}

function teardown() {
	cleanup_test
}

# 1. test running with loading the default apparmor profile.
# test that we can run with the default apparmor profile which will not block touching a file in `.`
@test "load default apparmor profile and run a container with it" {
	start_crio

	jq '	  .linux.security_context.apparmor_profile = "runtime/default"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/apparmor1.json
	pod_id=$(crictl runp "$TESTDIR"/apparmor1.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/apparmor1.json)

	crictl exec --sync "$ctr_id" touch test.txt
}

# 2. test running with loading a specific apparmor profile as crio default apparmor profile.
# test that we can run with a specific apparmor profile which will block touching a file in `.` as crio default apparmor profile.
@test "load a specific apparmor profile as default apparmor and run a container with it" {
	load_apparmor_profile "$APPARMOR_TEST_PROFILE_PATH"
	start_crio "$APPARMOR_TEST_PROFILE_NAME"

	jq '	  .linux.security_context.apparmor_profile = "apparmor-test-deny-write"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/apparmor2.json
	jq '	  .linux.security_context.apparmor_profile = "apparmor-test-deny-write"' \
		"$TESTDATA"/container_redis.json > "$TESTDIR"/apparmor_container2.json

	pod_id=$(crictl runp "$TESTDIR"/apparmor2.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/apparmor_container2.json "$TESTDIR"/apparmor2.json)

	run crictl exec --sync "$ctr_id" touch test.txt
	[[ "$output" == *"Permission denied"* ]]

	remove_apparmor_profile "$APPARMOR_TEST_PROFILE_PATH"
}

# 3. test running with loading a specific apparmor profile but not as crio default apparmor profile.
# test that we can run with a specific apparmor profile which will block touching a file in `.`
@test "load default apparmor profile and run a container with another apparmor profile" {
	load_apparmor_profile "$APPARMOR_TEST_PROFILE_PATH"
	start_crio

	jq '	  .linux.security_context.apparmor_profile = "apparmor-test-deny-write"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/apparmor3.json
	jq '	  .linux.security_context.apparmor_profile = "apparmor-test-deny-write"' \
		"$TESTDATA"/container_redis.json > "$TESTDIR"/apparmor_container3.json

	pod_id=$(crictl runp "$TESTDIR"/apparmor3.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/apparmor_container3.json "$TESTDIR"/apparmor3.json)

	run crictl exec --sync "$ctr_id" touch test.txt
	[[ "$output" == *"Permission denied"* ]]

	remove_apparmor_profile "$APPARMOR_TEST_PROFILE_PATH"
}

# 4. test running with wrong apparmor profile name.
# test that we will fail when running a ctr with wrong apparmor profile name.
@test "run a container with wrong apparmor profile name" {
	start_crio

	jq '	  .linux.security_context.apparmor_profile = "not-exists"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/apparmor4.json

	jq '	  .linux.security_context.apparmor_profile = "not-exists"' \
		"$TESTDATA"/container_redis.json > "$TESTDIR"/apparmor_container4.json

	pod_id=$(crictl runp "$TESTDIR"/apparmor4.json)

	run ! crictl create "$pod_id" "$TESTDIR"/apparmor_container4.json "$TESTDIR"/apparmor4.json
}

# 5. test running with default apparmor profile unloaded.
# test that we will fail when running a ctr with wrong apparmor profile name.
@test "run a container after unloading default apparmor profile" {
	load_apparmor_profile "$FAKE_CRIO_DEFAULT_PROFILE_PATH"
	start_crio "$FAKE_CRIO_DEFAULT_PROFILE_NAME"
	remove_apparmor_profile "$FAKE_CRIO_DEFAULT_PROFILE_PATH"

	jq '	  .linux.security_context.apparmor_profile = "runtime/default"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/apparmor5.json
	jq '	  .linux.security_context.apparmor_profile = "runtime/default"' \
		"$TESTDATA"/container_redis.json > "$TESTDIR"/apparmor_container5.json

	pod_id=$(crictl runp "$TESTDIR"/apparmor5.json)

	run ! crictl create "$pod_id" "$TESTDIR"/apparmor_container5.json "$TESTDIR"/apparmor5.json
}

# 6. test running with empty localhost profile name.
@test "run a container with invalid localhost apparmor profile name" {
	start_crio

	jq '.linux.security_context.apparmor_profile = "localhost/"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/apparmor4.json

	jq '.linux.security_context.apparmor_profile = "localhost/"' \
		"$TESTDATA"/container_redis.json > "$TESTDIR"/apparmor_container4.json

	pod_id=$(crictl runp "$TESTDIR"/apparmor4.json)

	run ! crictl create "$pod_id" "$TESTDIR"/apparmor_container4.json "$TESTDIR"/apparmor4.json
}
