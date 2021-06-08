#!/usr/bin/env bats

load helpers

function setup() {
	export activation="workload.openshift.io/management"
	export prefix="io.openshift.workload.management"
	setup_test
	sboxconfig="$TESTDIR/sbox.json"
	ctrconfig="$TESTDIR/ctr.json"
	systemd_supports_cpuset=$(systemctl show --property=AllowedCPUs systemd || true)
	export systemd_supports_cpuset
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
[crio.runtime.workloads.management.resources]
cpushares =  $cpushares
cpuset = "$cpuset"
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

function check_conmon_fields() {
	local ctr_id="$1"
	local cpushares="$2"
	local cpuset="$3"

	if [[ "$CONTAINER_CGROUP_MANAGER" == "cgroupfs" ]]; then
		if is_cgroup_v2; then
			cpuset_path="/sys/fs/cgroup"
			cpushare_path="/sys/fs/cgroup"
			cpushare_filename="cpu.weight"
			# see https://github.com/containers/crun/blob/e5874864918f8f07acdff083f83a7a59da8abb72/crun.1.md#cpu-controller for conversion
			cpushares=$((1 + ((cpushares - 2) * 9999) / 262142))
		else
			cpuset_path="/sys/fs/cgroup/cpuset"
			cpushare_path="/sys/fs/cgroup/cpu"
			cpushare_filename="cpu.shares"
		fi

		found_cpuset=$(cat "$cpuset_path/pod_123-456/crio-conmon-$ctr_id/cpuset.cpus")
		if [ -z "$cpuset" ]; then
			[[ $(cat "$cpuset_path/pod_123-456/cpuset.cpus") == *"$found_cpuset"* ]]
		else
			[[ "$cpuset" == *"$found_cpuset"* ]]
		fi

		found_cpushares=$(cat "$cpushare_path/pod_123-456/crio-conmon-$ctr_id/$cpushare_filename")
		if [ -z "$cpushares" ]; then
			[[ $(cat "$cpushare_path/pod_123-456/$cpushare_filename") == *"$found_cpushares"* ]]
		else
			[[ "$cpushares" == *"$found_cpushares"* ]]
		fi
	else
		# don't test cpuset if it's not supported by systemd
		if [[ -n "$systemd_supports_cpuset" ]]; then
			info="$(systemctl show --property=AllowedCPUs crio-conmon-"$ctr_id".scope)"
			if [ -z "$cpuset" ]; then
				echo "$info" | grep -E '^AllowedCPUs=$'
			else
				[[ "$info" == *"AllowedCPUs=$cpuset"* ]]
			fi
		fi

		info="$(systemctl show --property=CPUShares crio-conmon-"$ctr_id".scope)"
		if [ -z "$cpushares" ]; then
			# 18446744073709551615 is 2^64-1, which is the default systemd set in RHEL 7
			echo "$info" | grep -E '^CPUShares=\[not set\]$' || echo "$info" | grep 'CPUShares=18446744073709551615'
		else
			[[ "$info" == *"CPUShares=$cpushares"* ]]
		fi
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
	create_workload "$shares" "0"

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

@test "test workload should not be set if not defaulted or specified" {
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

@test "test workload should not be set if annotation not specified" {
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

@test "test workload pod gets configured to defaults" {
	shares="200"
	set="0-1"
	create_workload "$shares" "$set"

	start_crio

	jq --arg act "$activation" ' .annotations[$act] = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg act "$activation" ' .annotations[$act] = "true"' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	check_conmon_fields "$ctr_id" "$shares" "$set"
}

@test "test workload can override pod defaults" {
	shares="200"
	set="0-1"
	name=POD
	create_workload "$shares" "0"

	start_crio

	jq --arg act "$activation" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$act] = "true"
		|   .annotations[$setkey] = $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg act "$activation" --arg name "$name" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$act] = "true"
		|   .annotations[$setkey] = $set' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	check_conmon_fields "$ctr_id" "$shares" "$set"
}

@test "test workload pod should not be set if not defaulted or specified" {
	shares="200"
	set=""
	name=POD
	create_workload "$shares" ""

	start_crio

	jq --arg act "$activation" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$act] = "true"
		|   .annotations[$setkey] = $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg act "$activation" --arg name "$name" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$act] = "true"
		|   .annotations[$setkey] = $set' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	check_conmon_fields "$ctr_id" "$shares" "$set"
}

@test "test workload pod should not be set if annotation not specified" {
	shares=""
	set=""
	name=POD
	create_workload "200" "0-1"

	start_crio

	jq --arg act "$activation" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$setkey] = $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq --arg act "$activation" --arg name "$name" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$setkey] = $set
		|   del(.linux.resources.cpu_shares)' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	check_conmon_fields "$ctr_id" "$shares" "$set"
}
