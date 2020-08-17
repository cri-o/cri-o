#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

@test "version should succeed" {
    # when
    run $CRIO_BINARY_PATH version
    echo "$output"

    # then
    [ "$status" -eq 0 ]
    [[ "$output" == *"Version:"* ]]
    [[ "$output" == *"GitCommit:"* ]]
    [[ "$output" == *"GitTreeState:"* ]]
    if [[ "$output" == *"Linkmode: dynamic"* ]]; then
        [[ "$output" == *"BuildDate:"* ]]
    fi
    [[ "$output" == *"GoVersion:"* ]]
    [[ "$output" == *"Compiler:"* ]]
    [[ "$output" == *"Platform:"* ]]
    [[ "$output" == *"Linkmode:"* ]]
}

@test "version should succeed with JSON" {
    # when
    run $CRIO_BINARY_PATH version -j
    echo "$output"

    # then
    JSON="$output"
    echo $JSON | jq --exit-status '.gitCommit != ""'
    [ "$status" -eq 0 ]

    if echo $JSON | jq -r '.linkmode == "dynamic"'; then
        echo $JSON | jq --exit-status '.buildDate != ""'
    fi

    echo $JSON | jq --exit-status '.goVersion != ""'
    [ "$status" -eq 0 ]

    echo $JSON | jq --exit-status '.compiler != ""'
    [ "$status" -eq 0 ]

    echo $JSON | jq --exit-status '.platform != ""'
    [ "$status" -eq 0 ]

    echo $JSON | jq --exit-status '.gitTreeState != ""'
    [ "$status" -eq 0 ]

    echo $JSON | jq --exit-status '.version != ""'
    [ "$status" -eq 0 ]

    echo $JSON | jq --exit-status '.linkmode != ""'
    [ "$status" -eq 0 ]
}
