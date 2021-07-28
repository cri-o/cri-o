#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

@test "version" {
	# when
	output=$("$CRIO_BINARY_PATH" version)
	echo "$output"

	# then
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
	[[ "$output" == *"BuildTags:"* ]]
	[[ "$output" == *"SeccompEnabled:"* ]]
	[[ "$output" == *"AppArmorEnabled:"* ]]
}

@test "version -j" {
	# when
	JSON=$("$CRIO_BINARY_PATH" version -j)
	echo "$JSON"

	# then
	[ -n "$JSON" ]

	echo "$JSON" | jq -e '.gitCommit != ""'

	if echo "$JSON" | jq -e '.linkmode == "dynamic"'; then
		echo "$JSON" | jq -e '.buildDate != ""'
	fi

	echo "$JSON" | jq -e '.goVersion != ""'
	echo "$JSON" | jq -e '.compiler != ""'
	echo "$JSON" | jq -e '.platform != ""'
	echo "$JSON" | jq -e '.gitTreeState != ""'
	echo "$JSON" | jq -e '.version != ""'
	echo "$JSON" | jq -e '.linkmode != ""'
	echo "$JSON" | jq -e '.buildTags != ""'
	echo "$JSON" | jq -e '.seccompEnabled != ""'
	echo "$JSON" | jq -e '.appArmorEnabled != ""'
}
