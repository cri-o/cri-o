#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

function run_pod() {
	crictl runp "$TESTDATA"/sandbox_config.json
}

function delete_pod() {
	id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl stopp "$id"
	crictl rmp "$id"
}

@test "single cni plugin with pod annotations capability enabled" {
	enable_capability=true
	cni_plugin_log_path="$TESTDIR/cni.log"

	start_crio
	prepare_cni_plugin "$cni_plugin_log_path" $enable_capability
	run_pod
	delete_pod

	annotations_cni_plugin_received=$(jq '.runtimeConfig."io.kubernetes.cri.pod-annotations"' "$cni_plugin_log_path")
	expected=$(jq '.annotations' "$TESTDATA/sandbox_config.json")

	annotations_equal "$annotations_cni_plugin_received" "$expected"
}

@test "single cni plugin with pod annotations capability disabled" {
	enable_capability=false
	cni_plugin_log_path="$TESTDIR/cni.log"

	start_crio
	prepare_cni_plugin "$cni_plugin_log_path" $enable_capability
	run_pod
	delete_pod

	annotations_cni_plugin_received=$(jq '.runtimeConfig."io.kubernetes.cri.pod-annotations"' "$cni_plugin_log_path")
	expected=null

	annotations_equal "$annotations_cni_plugin_received" "$expected"
}

@test "pod annotations capability for chained cni plugins" {
	enable_capability_first=false
	cni_plugin_log_path_first="$TESTDIR/cni-01.log"
	enable_capability_second=true
	cni_plugin_log_path_second="$TESTDIR/cni-02.log"

	start_crio
	prepare_chained_cni_plugins "${cni_plugin_log_path_first}" $enable_capability_first "$cni_plugin_log_path_second" $enable_capability_second
	run_pod
	delete_pod

	annotations_first_cni_plugin_received=$(jq '.runtimeConfig."io.kubernetes.cri.pod-annotations"' "$cni_plugin_log_path_first")
	if [[ $enable_capability_first == true ]]; then
		expected=$(jq '.annotations' "$TESTDATA/sandbox_config.json")
	else
		expected=null
	fi
	annotations_equal "$annotations_first_cni_plugin_received" "$expected"

	annotations_second_cni_plugin_received=$(jq '.runtimeConfig."io.kubernetes.cri.pod-annotations"' "$cni_plugin_log_path_second")
	if [[ $enable_capability_second == true ]]; then
		expected=$(jq '.annotations' "$TESTDATA/sandbox_config.json")
	else
		expected=null
	fi
	annotations_equal "$annotations_second_cni_plugin_received" "$expected"
}
