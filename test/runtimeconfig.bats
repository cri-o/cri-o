#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "crictl info prints out the crio runtime config" {
	start_crio
	info_json=$(crictl info -o json)

	jq -e '.config.crio' <<< "$info_json"

	default_runtime=$(jq -r ".config.crio.DefaultRuntime" <<< "$info_json")
	jq -e ".config.crio.Runtimes.[\"$default_runtime\"]" <<< "$info_json"

	crio_runtime_bin_path=$(jq -r ".config.crio.Runtimes.[\"$default_runtime\"].RuntimePath" <<< "$info_json")
	[[ "${crio_runtime_bin_path}" == "$RUNTIME_BINARY_PATH" ]]
}
