#!/usr/bin/env bats
# vim:set ft=bash :

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
			cpushare_path="/sys/fs/cgroup"
			cpushare_filename="cpu.weight"
			# see https://github.com/containers/crun/blob/e5874864918f8f07acdff083f83a7a59da8abb72/crun.1.md#cpu-controller for conversion
			cpushares=$((1 + ((cpushares - 2) * 9999) / 262142))
		else
			cpushare_path="/sys/fs/cgroup/cpu"
			cpushare_filename="cpu.shares"
		fi

		found_cpushares=$(cat "$cpushare_path/pod_123-456/crio-conmon-$ctr_id/$cpushare_filename")
		if [ -z "$cpushares" ]; then
			[[ $(cat "$cpushare_path/pod_123-456/$cpushare_filename") == *"$found_cpushares"* ]]
		else
			[[ "$cpushares" == *"$found_cpushares"* ]]
		fi
	else
		info="$(systemctl show --property=CPUShares crio-conmon-"$ctr_id".scope)"
		if [ -z "$cpushares" ]; then
			# 18446744073709551615 is 2^64-1, which is the default systemd set in RHEL 7
			echo "$info" | grep -E '^CPUShares=\[not set\]$' || echo "$info" | grep 'CPUShares=18446744073709551615'
		else
			[[ "$info" == *"CPUShares=$cpushares"* ]]
		fi
	fi
	check_conmon_cpuset "$ctr_id" "$cpuset"
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
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

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
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

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
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

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

@test "test workload pod should override infra_ctr_cpuset option" {
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

	shares="200"
	set="0-1"
	name=POD
	create_workload "$shares" "0"

	CONTAINER_INFRA_CTR_CPUSET="1" start_crio

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

@test "test workload allowed annotation should not work if not configured" {
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.ShmSize" "$activation"

	start_crio

	jq '.annotations."io.kubernetes.cri-o.ShmSize" = "16Mi"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	ctrconfig="$TESTDATA"/container_sleep.json
	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	df=$(crictl exec --sync "$ctr_id" df | grep /dev/shm)
	[[ "$df" != *'16384'* ]]
}

@test "test workload allowed annotation appended with runtime" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.Devices"
	create_runtime_with_allowed_annotation "shmsize" "io.kubernetes.cri-o.ShmSize"

	CONTAINER_ALLOWED_DEVICES="/dev/null" start_crio

	jq --arg act "$activation" \
		'   .annotations."io.kubernetes.cri-o.ShmSize" = "16Mi"
	    |   .annotations."io.kubernetes.cri-o.Devices" = "/dev/null:/dev/peterfoo:rwm"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	ctrconfig="$TESTDATA"/container_sleep.json
	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	df=$(crictl exec --sync "$ctr_id" df | grep /dev/shm)
	[[ "$df" == *'16384'* ]]

	crictl exec --sync "$ctr_id" sh -c "head -n1 /dev/peterfoo"

}

@test "test workload allowed annotation works for pod" {
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.ShmSize"

	name=POD
	start_crio

	jq --arg act "$activation" \
		' .annotations."io.kubernetes.cri-o.ShmSize" = "16Mi"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	ctrconfig="$TESTDATA"/container_sleep.json
	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	df=$(crictl exec --sync "$ctr_id" df | grep /dev/shm)
	[[ "$df" == *'16384'* ]]
}

@test "test resource cleanup on bad annotation contents" {
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

	shares="200"
	set="0-2"
	name=helloctr
	create_workload "$shares" "0"

	start_crio

	jq --arg act "$activation" --arg set "{\"cpuset\": \"$set\"}" --arg setkey "$prefix/$name" \
		'   .annotations[$act] = "true"
		|   .annotations[$setkey] = $set' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"
	pod_id=$(crictl runp "$sboxconfig")

	# Forceibly fail 10 container creations via bad workload annotation:
	for id in {1..10}; do
		ctrconfig="$TESTDIR/ctr-$id.json"
		jq --arg act "$activation" --arg name "$name-$id" --arg set "invalid & unparsable {\"cpuset\": \"$set\"}" --arg setkey "$prefix/POD" \
			'   .annotations[$act] = "true"
			|   .annotations[$setkey] = $set
			|   .metadata.name = $name' \
			"$TESTDATA"/container_sleep.json > "$ctrconfig"
		ctr_id=$(crictl create "$pod_id" "$ctrconfig" "$sboxconfig" || true)
		[[ $ctr_id == "" ]]
	done
	# Ensure there are no conmon zombies leftover
	children=$(ps --ppid "$CRIO_PID" -o state= -o pid= -o cmd= || true)
	zombies=$(grep -c '^Z ' <<< "$children" || true)
	echo "Zombies: $zombies"
	[[ $zombies == 0 ]]
}

@test "test workload pod should not be set if annotation not specified even if prefix" {
	start_crio

	jq '   .annotations["io.kubernetes.cri-o.UnifiedCgroup.podsandbox-sleep"] = "memory.max=4294967296" |
	  .labels["io.kubernetes.container.name"] = "podsandbox-sleep"' \
	"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq '   .annotations["io.kubernetes.cri-o.UnifiedCgroup.podsandbox-sleep"] = "memory.max=4294967296" |
	  .labels["io.kubernetes.container.name"] = "podsandbox-sleep"' \
	"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")
	[[ $(crictl exec "$ctr_id" cat /sys/fs/cgroup/memory.max) != 4294967296 ]]
}
