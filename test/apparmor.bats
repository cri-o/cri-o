#!/usr/bin/env bats

load helpers

function teardown() {
    cleanup_test
}

# AppArmor tests have to run in sequence since they modify the system state
@test "apparmor tests (in sequence)" {
    if [[ $(is_apparmor_enabled) -eq 0 ]]; then
        skip "skip test since apparmor is not enabled."
    fi

    load_default_apparmor_profile_and_run_a_container_with_it
    load_a_specific_apparmor_profile_as_default_apparmor_and_run_a_container_with_it
    load_default_apparmor_profile_and_run_a_container_with_another_apparmor_profile
    run_a_container_with_wrong_apparmor_profile_name
    run_a_container_after_unloading_default_apparmor_profile
}

# 1. test running with loading the default apparmor profile.
# test that we can run with the default apparmor profile which will not block touching a file in `.`
load_default_apparmor_profile_and_run_a_container_with_it() {
    local output status

    setup_test
    start_crio

    sed -e 's/%VALUE%/runtime\/default/g' "$TESTDATA"/sandbox_config_apparmor.json > "$TESTDIR"/apparmor1.json

    pod_id=$(crictl runp "$TESTDIR"/apparmor1.json)
    ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/apparmor1.json)

    crictl exec --sync "$ctr_id" touch test.txt

    cleanup_test
}

# 2. test running with loading a specific apparmor profile as crio default apparmor profile.
# test that we can run with a specific apparmor profile which will block touching a file in `.` as crio default apparmor profile.
load_a_specific_apparmor_profile_as_default_apparmor_and_run_a_container_with_it() {
    local output status

    setup_test
    load_apparmor_profile "$APPARMOR_TEST_PROFILE_PATH"
    start_crio "$APPARMOR_TEST_PROFILE_NAME"

    sed -e 's/%VALUE%/apparmor-test-deny-write/g' "$TESTDATA"/sandbox_config_apparmor.json > "$TESTDIR"/apparmor2.json
    sed -e 's/%VALUE%/apparmor-test-deny-write/g' "$TESTDATA"/container_redis_apparmor.json > "$TESTDIR"/apparmor_container2.json

    pod_id=$(crictl runp "$TESTDIR"/apparmor2.json)
    ctr_id=$(crictl create "$pod_id" "$TESTDIR"/apparmor_container2.json "$TESTDIR"/apparmor2.json)

    run crictl exec --sync "$ctr_id" touch test.txt
    echo "$output"
    [[ "$output" == *"Permission denied"* ]]

    remove_apparmor_profile "$APPARMOR_TEST_PROFILE_PATH"
    cleanup_test
}

# 3. test running with loading a specific apparmor profile but not as crio default apparmor profile.
# test that we can run with a specific apparmor profile which will block touching a file in `.`
load_default_apparmor_profile_and_run_a_container_with_another_apparmor_profile() {
    local output status

    setup_test
    load_apparmor_profile "$APPARMOR_TEST_PROFILE_PATH"
    start_crio

    sed -e 's/%VALUE%/apparmor-test-deny-write/g' "$TESTDATA"/sandbox_config_apparmor.json > "$TESTDIR"/apparmor3.json
    sed -e 's/%VALUE%/apparmor-test-deny-write/g' "$TESTDATA"/container_redis_apparmor.json > "$TESTDIR"/apparmor_container3.json

    pod_id=$(crictl runp "$TESTDIR"/apparmor3.json)
    ctr_id=$(crictl create "$pod_id" "$TESTDIR"/apparmor_container3.json "$TESTDIR"/apparmor3.json)

    run crictl exec --sync "$ctr_id" touch test.txt
    echo "$output"
    [[ "$output" == *"Permission denied"* ]]

    remove_apparmor_profile "$APPARMOR_TEST_PROFILE_PATH"
    cleanup_test
}

# 4. test running with wrong apparmor profile name.
# test that we will fail when running a ctr with wrong apparmor profile name.
run_a_container_with_wrong_apparmor_profile_name() {
    local output status

    setup_test
    start_crio

    sed -e 's/%VALUE%/not-exists/g' "$TESTDATA"/sandbox_config_apparmor.json > "$TESTDIR"/apparmor4.json
    sed -e 's/%VALUE%/not-exists/g' "$TESTDATA"/container_redis_apparmor.json > "$TESTDIR"/apparmor_container4.json

    pod_id=$(crictl runp "$TESTDIR"/apparmor4.json)

    run crictl create "$pod_id" "$TESTDIR"/apparmor_container4.json "$TESTDIR"/apparmor4.json
    echo "$output"
    [ "$status" -ne 0 ]

    cleanup_test
}

# 5. test running with default apparmor profile unloaded.
# test that we will fail when running a ctr with wrong apparmor profile name.
run_a_container_after_unloading_default_apparmor_profile() {
    local output status

    load_apparmor_profile "$FAKE_CRIO_DEFAULT_PROFILE_PATH"
    setup_test
    start_crio "$FAKE_CRIO_DEFAULT_PROFILE_NAME"
    remove_apparmor_profile "$FAKE_CRIO_DEFAULT_PROFILE_PATH"

    sed -e 's/%VALUE%/runtime\/default/g' "$TESTDATA"/sandbox_config_apparmor.json > "$TESTDIR"/apparmor5.json
    sed -e 's/%VALUE%/runtime\/default/g' "$TESTDATA"/container_redis_apparmor.json > "$TESTDIR"/apparmor_container5.json

    pod_id=$(crictl runp "$TESTDIR"/apparmor5.json)

    crictl create "$pod_id" "$TESTDIR"/apparmor_container5.json "$TESTDIR"/apparmor5.json && fail

    cleanup_test
}
