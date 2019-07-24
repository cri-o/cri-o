#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
    start_crio

    # the default log_level is `error` so we have to adapt it before running
    # any tests to be able to see the `info` messages
    replace_config "log_level" "debug"
}

function teardown() {
    cleanup_test
}

function replace_config() {
    sed -ie 's;\('$1' = "\).*\("\);\1'$2'\2;' "$CRIO_CONFIG"
}

function reload_crio() {
    kill -HUP $CRIO_PID
}

function wait_for_log() {
    CNT=0
    while true; do
        if [[ $CNT -gt 50 ]]; then
            echo wait for log timed out
            exit 1
        fi

        if grep -q "$1" "$CRIO_LOG"; then
            break
        fi

        echo "waiting for log entry to appear ($CNT): $1"
        sleep 0.1
        CNT=$((CNT + 1))
    done
}

function expect_log_success() {
    wait_for_log '"set config '$1' to \\"'$2'\\""'
}

function expect_log_failure() {
    wait_for_log "unable to reload configuration: $1"
}

@test "should succeed to reload" {
    # when
    reload_crio

    # then
    ps --pid $CRIO_PID &>/dev/null
}

@test "should succeed to reload 'log_level'" {
    # given
    NEW_LEVEL="warn"
    OPTION="log_level"

    # when
    replace_config $OPTION $NEW_LEVEL
    reload_crio

    # then
    expect_log_success $OPTION $NEW_LEVEL
}

@test "should fail to reload 'log_level' if invalid" {
    # when
    replace_config "log_level" "invalid"
    reload_crio

    # then
    expect_log_failure "not a valid logrus Level"
}


@test "should fail to reload if config is malformed" {
    # when
    replace_config "log_level" '\"'
    reload_crio

    # then
    expect_log_failure "unable to decode configuration"
}

@test "should succeed to reload 'pause_image'" {
    # given
    NEW_OPTION="new-image"
    OPTION="pause_image"

    # when
    replace_config $OPTION $NEW_OPTION
    reload_crio

    # then
    expect_log_success $OPTION $NEW_OPTION
}

@test "should succeed to reload 'pause_command'" {
    # given
    NEW_OPTION="new-command"
    OPTION="pause_command"

    # when
    replace_config $OPTION $NEW_OPTION
    reload_crio

    # then
    expect_log_success $OPTION $NEW_OPTION
}

@test "should succeed to reload 'pause_image_auth_file'" {
    # given
    NEW_OPTION="$TESTDIR/auth_file"
    OPTION="pause_image_auth_file"
    touch $NEW_OPTION

    # when
    replace_config $OPTION $NEW_OPTION
    reload_crio

    # then
    expect_log_success $OPTION $NEW_OPTION
}

@test "should fail to reload non existing 'pause_image_auth_file'" {
    # given
    NEW_OPTION="$TESTDIR/auth_file"
    OPTION="pause_image_auth_file"

    # when
    replace_config $OPTION $NEW_OPTION
    reload_crio

    # then
    expect_log_failure "stat $NEW_OPTION"
}
