#!/usr/bin/env bats

load helpers

function setup() {
	export activation="workload.openshift.io/management"
	export prefix="io.openshift.workload.management"
	setup_test
	sboxconfig="$TESTDIR/sbox.json"
	ctrconfig="$TESTDIR/ctr.json"
}

function teardown() {
	cleanup_test
}

function create_workload() {
	local cpushares="$1"
	local cpuset="$2"
	cat << EOF > "$CRIO_CONFIG_DIR/01-workload.conf"
[crio.runtime.workloads.management]
activation_annotation = "$activation"
annotation_prefix = "$prefix"
resources = { "cpushares" =  "$cpushares", "cpuset" = "$cpuset" }
EOF
}

function check_cpu_fields() {
	local ctr_id="$1"
	local cpushares="$2"
	local cpuset="$3"

	config=$(runtime state "$ctr_id" | jq -r .bundle)/config.json

	if [ -z "$cpushares" ]; then
		[[ $(jq -r .linux.resources.cpu.shares < "$config") == 0 ]]
	else
		[[ $(jq .linux.resources.cpu.shares < "$config") == *"$cpushares"* ]]
	fi

	if [ -z "$cpuset" ]; then
		[[ $(jq -r .linux.resources.cpu.cpus < "$config") == null ]]
	else
		[[ $(jq .linux.resources.cpu.cpus < "$config") == *"$cpuset"* ]]
	fi
}

@test "test workload gets configured to defaults" {
	shares="200"
	set="0-1"
	create_workload "$shares" "$set"

	start_crio

	jq --arg act "$activation" ' .annotations[$act] = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg act "$activation" ' .annotations[$act] = "true"' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	check_cpu_fields "$ctr_id" "$shares" "$set"
}

@test "test workload can override defaults" {
	shares="200"
	set="0-1"
	name=helloctr
	create_workload "$shares" "0-2"

	start_crio

	jq --arg act "$activation" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$act] = "true"
		|   .annotations[$setkey] = $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg act "$activation" --arg name "$name" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$act] = "true"
		|   .annotations[$setkey] = $set
		|   .metadata.name = $name' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	check_cpu_fields "$ctr_id" "$shares" "$set"
}

@test "test workload should not set if not defaulted or specified" {
	shares="200"
	set=""
	name=helloctr
	create_workload "$shares" ""

	start_crio

	jq --arg act "$activation" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$act] = "true"
		|   .annotations[$setkey] = $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg act "$activation" --arg name "$name" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$act] = "true"
		|   .annotations[$setkey] = $set
		|   .metadata.name = $name' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	check_cpu_fields "$ctr_id" "$shares" "$set"
}

@test "test workload should not set if annotation not specified" {
	shares=""
	set=""
	name=helloctr
	create_workload "200" "0-1"

	start_crio

	jq --arg act "$activation" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$setkey] = $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg act "$activation" --arg name "$name" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$setkey] = $set
		|   .metadata.name = $name
		|   del(.linux.resources.cpu_shares)' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	check_cpu_fields "$ctr_id" "$shares" "$set"
}
