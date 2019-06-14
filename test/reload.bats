#!/usr/bin/env bats

load helpers

function setup() {
    start_crio
}

function teardown() {
    cleanup_test
}

function replace_config() {
    sed -ie 's/\('$1' = "\).*\("\)/\1'$2'\2/' "$CRIO_CONFIG"
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
