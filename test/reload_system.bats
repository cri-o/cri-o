#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
    setup_test
    export TEST_IMAGE=quay.io/saschagrunert/hello-world \
           CONTAINER_REGISTRIES_CONF="$TESTDIR/containers/registries.conf"
    printf "[[registry]]\nlocation = 'quay.io/saschagrunert'\nblocked = true" \
        >> $CONTAINER_REGISTRIES_CONF
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

function expect_pull_image() {
    run crictl pull "$TEST_IMAGE"
    echo "$output"
    [ "$status" -eq $1 ]
}

@test "reload system registries should succeed" {
    # given
    start_crio
    replace_config "log_level" "debug"

    # when
    reload_crio

    # then
    expect_log_success
    expect_pull_image 1
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
    expect_pull_image 0
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
