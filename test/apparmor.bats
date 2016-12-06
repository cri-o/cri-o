#!/usr/bin/env bats

load helpers

function teardown() {
    cleanup_test
}

# 1. test running with loading the default apparmor profile.
# test that we can run with the default apparmor profile which will not block touching a file in `.`
@test "load default apparmor profile and run a container with it" {
    # this test requires docker, thus it can't yet be run in a container
    if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
        skip "cannot yet run this test in a container, use sudo make localintegration"
    fi

    # this test requires apparmor, so skip this test if apparmor is not enabled.
    enabled=is_apparmor_enabled
    if [[ "$enabled" = "0" ]]; then
        skip "skip this test since apparmor is not enabled."
    fi

    start_ocid

    sed -e 's/%VALUE%/,"container\.apparmor\.security\.beta\.kubernetes\.io\/testname1": "runtime\/default"/g' "$TESTDATA"/sandbox_config_seccomp.json > "$TESTDIR"/apparmor1.json

    run ocic pod create --name apparmor1 --config "$TESTDIR"/apparmor1.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"
    run ocic ctr create --name testname1 --config "$TESTDATA"/container_redis.json --pod "$pod_id"
    echo "$output"
    [ "$status" -eq 0 ]
    ctr_id="$output"
    [ "$status" -eq 0 ]
    run ocic ctr execsync --id "$ctr_id" touch test.txt
    echo "$output"
    [ "$status" -eq 0 ]


    cleanup_ctrs
    cleanup_pods
    stop_ocid
}

# 2. test running with loading a specific apparmor profile as ocid default apparmor profile.
# test that we can run with a specific apparmor profile which will block touching a file in `.` as ocid default apparmor profile.
@test "load a specific apparmor profile as default apparmor and run a container with it" {
    # this test requires docker, thus it can't yet be run in a container
    if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
        skip "cannot yet run this test in a container, use sudo make localintegration"
    fi

    # this test requires apparmor, so skip this test if apparmor is not enabled.
    enabled=is_apparmor_enabled
    if [[ "$enabled" -eq "0" ]]; then
        skip "skip this test since apparmor is not enabled."
    fi

    load_apparmor_test_profile
    start_ocid "" "$APPARMOR_TEST_PROFILE_NAME"

    sed -e 's/%VALUE%/,"container\.apparmor\.security\.beta\.kubernetes\.io\/testname2": "apparmor-test-deny-write"/g' "$TESTDATA"/sandbox_config_seccomp.json > "$TESTDIR"/apparmor2.json

    run ocic pod create --name apparmor2 --config "$TESTDIR"/apparmor2.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"
    run ocic ctr create --name testname2 --config "$TESTDATA"/container_redis.json --pod "$pod_id"
    echo "$output"
    [ "$status" -eq 0 ]
    ctr_id="$output"
    [ "$status" -eq 0 ]
    run ocic ctr execsync --id "$ctr_id" touch test.txt
    echo "$output"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Permission denied" ]]

    cleanup_ctrs
    cleanup_pods
    stop_ocid
    remove_apparmor_test_profile
}

# 3. test running with loading a specific apparmor profile but not as ocid default apparmor profile.
# test that we can run with a specific apparmor profile which will block touching a file in `.`
@test "load default apparmor profile and run a container with another apparmor profile" {
    # this test requires docker, thus it can't yet be run in a container
    if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
        skip "cannot yet run this test in a container, use sudo make localintegration"
    fi

    # this test requires apparmor, so skip this test if apparmor is not enabled.
    enabled=is_apparmor_enabled
    if [[ "$enabled" -eq "0" ]]; then
        skip "skip this test since apparmor is not enabled."
    fi

    load_apparmor_test_profile
    start_ocid

    sed -e 's/%VALUE%/,"container\.apparmor\.security\.beta\.kubernetes\.io\/testname3": "apparmor-test-deny-write"/g' "$TESTDATA"/sandbox_config_seccomp.json > "$TESTDIR"/apparmor3.json

    run ocic pod create --name apparmor3 --config "$TESTDIR"/apparmor3.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"
    run ocic ctr create --name testname3 --config "$TESTDATA"/container_redis.json --pod "$pod_id"
    echo "$output"
    [ "$status" -eq 0 ]
    ctr_id="$output"
    [ "$status" -eq 0 ]
    run ocic ctr execsync --id "$ctr_id" touch test.txt
    echo "$output"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Permission denied" ]]

    cleanup_ctrs
    cleanup_pods
    stop_ocid
    remove_apparmor_test_profile
}

# 4. test running with wrong apparmor profile name.
# test that we can will fail when running a ctr with rong apparmor profile name.
@test "run a container with wrong apparmor profile name" {
    # this test requires docker, thus it can't yet be run in a container
    if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
        skip "cannot yet run this test in a container, use sudo make localintegration"
    fi

    # this test requires apparmor, so skip this test if apparmor is not enabled.
    enabled=is_apparmor_enabled
    if [[ "$enabled" -eq "0" ]]; then
        skip "skip this test since apparmor is not enabled."
    fi

    start_ocid

    sed -e 's/%VALUE%/,"container\.apparmor\.security\.beta\.kubernetes\.io\/testname4": "not-exists"/g' "$TESTDATA"/sandbox_config_seccomp.json > "$TESTDIR"/apparmor4.json

    run ocic pod create --name apparmor4 --config "$TESTDIR"/apparmor4.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"
    run ocic ctr create --name testname4 --config "$TESTDATA"/container_redis.json --pod "$pod_id"
    echo "$output"
    [ "$status" -ne 0 ]
    [[ "$output" =~ "Creating container failed" ]]


    cleanup_ctrs
    cleanup_pods
    stop_ocid
}
