#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	newconfig="$TESTDIR/config.json"
	export RUNTIME_TYPE="spoofed"
}

function teardown() {
	cleanup_test
}

@test "ctr not found correct error message" {
	start_crio
	! crictl inspect "container_not_exist"
}


@test "ctr remove" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl rm -f "$ctr_id"
}

@test "ctr lifecycle" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	output=$(crictl pods --quiet)
	[[ "$output" == "$pod_id" ]]

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	output=$(crictl ps --quiet --state created)
	[[ "$output" == "$ctr_id" ]]

	output=$(crictl inspect "$ctr_id")
	[ -n "$output" ]
	echo "$output" | jq -e ".info.privileged == false"

	YEAR=$(date +"%Y")
	CREATED=$(echo "$output" | jq -re '.status.createdAt')
	echo "$output" | jq -e '.status.createdAt | startswith("'"$YEAR"'")'

	echo "$output" | jq -e '.status.startedAt | startswith("'"$YEAR"'") | not'
	echo "$output" | jq -e '.status.finishedAt | startswith("'"$YEAR"'") | not'

	crictl start "$ctr_id"
	output=$(crictl inspect "$ctr_id")
	[[ "$CREATED" == $(echo "$output" | jq -re '.status.createdAt') ]]

	STARTED=$(echo "$output" | jq -re '.status.startedAt')
	echo "$output" | jq -e '.status.startedAt | startswith("'"$YEAR"'")'

	echo "$output" | jq -e '.status.finishedAt | startswith("'"$YEAR"'") | not'

	output=$(crictl ps --quiet --state running)
	[[ "$output" == "$ctr_id" ]]

	crictl stop "$ctr_id"
	output=$(crictl inspect "$ctr_id")

	[[ "$CREATED" == $(echo "$output" | jq -re '.status.createdAt') ]]
	[[ "$STARTED" == $(echo "$output" | jq -re '.status.startedAt') ]]
	echo "$output" | jq -e '.status.finishedAt | startswith("'"$YEAR"'")'

	output=$(crictl ps --quiet --state exited)
	[[ "$output" == "$ctr_id" ]]

	crictl rm "$ctr_id"
	crictl ps --quiet
	crictl stopp "$pod_id"
	output=$(crictl pods --quiet)
	[[ "$output" == "$pod_id" ]]
	output=$(crictl ps --quiet)
	[[ "$output" == "" ]]

	crictl rmp "$pod_id"
	output=$(crictl pods --quiet)
	[[ "$output" == "" ]]
}


# regression test for #127
@test "ctrs status for a pod" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	output=$(crictl ps --quiet --state created)
	[ "$output" = "$ctr_id" ]

	printf '%s\n' "$output" | while IFS= read -r id; do
		crictl inspect "$id"
	done
}

@test "ctr list filtering" {
	# start 3 redis sandbox
	# pod1 ctr1 create & start
	# pod2 ctr2 create
	# pod3 ctr3 create & start & stop
	start_crio
	pod_config="$TESTDIR"/sandbox_config.json

	jq '	  .metadata.name = "podsandbox1"
		| .metadata.uid = "redhat-test-crio-1"' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"
	pod1_id=$(crictl runp "$pod_config")
	ctr1_id=$(crictl create "$pod1_id" "$TESTDATA"/container_redis.json "$pod_config")
	crictl start "$ctr1_id"

	jq '	  .metadata.name = "podsandbox2"
		| .metadata.uid = "redhat-test-crio-2"' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"
	pod2_id=$(crictl runp "$pod_config")
	ctr2_id=$(crictl create "$pod2_id" "$TESTDATA"/container_redis.json "$pod_config")

	jq '	  .metadata.name = "podsandbox3"
		| .metadata.uid = "redhat-test-crio-3"' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"
	pod3_id=$(crictl runp "$pod_config")
	ctr3_id=$(crictl create "$pod3_id" "$TESTDATA"/container_redis.json "$pod_config")
	crictl start "$ctr3_id"
	crictl stop "$ctr3_id"

	output=$(crictl ps --id "$ctr1_id" --quiet --all)
	[ "$output" = "$ctr1_id" ]

	output=$(crictl ps --id "${ctr1_id:0:4}" --quiet --all)
	[ "$output" = "$ctr1_id" ]

	output=$(crictl ps --id "$ctr2_id" --pod "$pod2_id" --quiet --all)
	[ "$output" = "$ctr2_id" ]

	output=$(crictl ps --id "$ctr2_id" --pod "$pod3_id" --quiet --all)
	[ "$output" = "" ]

	output=$(crictl ps --state created --quiet)
	[ "$output" = "$ctr2_id" ]

	output=$(crictl ps --state running --quiet)
	[ "$output" = "$ctr1_id" ]

	output=$(crictl ps --state exited --quiet)
	[ "$output" = "$ctr3_id" ]

	output=$(crictl ps --pod "$pod1_id" --quiet --all)
	[ "$output" = "$ctr1_id" ]

	output=$(crictl ps --pod "$pod2_id" --quiet --all)
	[ "$output" == "$ctr2_id" ]

	output=$(crictl ps --pod "$pod3_id" --quiet --all)
	[ "$output" == "$ctr3_id" ]
}

@test "ctr list label filtering" {
	# start a pod with 3 containers
	# ctr1 with labels: group=test container=redis version=v1.0.0
	# ctr2 with labels: group=test container=redis version=v1.0.0
	# ctr3 with labels: group=test container=redis version=v1.1.0
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .metadata.name = "ctr1"
		| .labels.group = "test"
		| .labels.name = "ctr1"
		| .labels.version = "v1.0.0"' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr1_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)

	jq '	  .metadata.name = "ctr2"
		| .labels.group = "test"
		| .labels.name = "ctr2"
		| .labels.version = "v1.0.0"' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr2_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)

	jq '	  .metadata.name = "ctr3"
		| .labels.group = "test"
		| .labels.name = "ctr3"
		| .labels.version = "v1.1.0"' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr3_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)

	output=$(crictl ps --label "group=test" --label "name=ctr1" --label "version=v1.0.0" --quiet --all)
	[ "$output" = "$ctr1_id" ]

	output=$(crictl ps --label "group=production" --quiet --all)
	[ "$output" == "" ]

	output=$(crictl ps --label "group=test" --label "version=v1.0.0" --quiet --all)
	[[ "$output" != "" ]]
	[[ "$output" == *"$ctr1_id"* ]]
	[[ "$output" == *"$ctr2_id"* ]]
	[[ "$output" != *"$ctr3_id"* ]]

	output=$(crictl ps --label "group=test" --quiet --all)
	[[ "$output" != "" ]]
	[[ "$output" == *"$ctr1_id"* ]]
	[[ "$output" == *"$ctr2_id"* ]]
	[[ "$output" == *"$ctr3_id"* ]]
}

@test "ctr metadata in list & status" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl ps --id "$ctr_id" --output yaml --state created)
	# TODO: expected value should not hard coded here
	[[ "$output" == *"name: container1"* ]]
	[[ "$output" == *"attempt: 1"* ]]

	output=$(crictl inspect -o table "$ctr_id")
	# TODO: expected value should not hard coded here
	[[ "$output" == *"Name: container1"* ]]
	[[ "$output" == *"Attempt: 1"* ]]
}

@test "ctr execsync" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" echo HELLO)
	[ "$output" = "" ]
}

# Devices are not emulated in spoofed runtime so these tests ensure devices are correctly spoofed
@test "ctr device add" {
	# In an user namespace we can only bind mount devices from the host, not mknod
	# https://github.com/opencontainers/runc/blob/master/libcontainer/rootfs_linux.go#L480-L481
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .devices = [ {
			host_path: "/dev/null",
			container_path: "/dev/mynull",
			permissions: "rwm"
		} ]
		| .linux.security_context.privileged = false' \
		"$TESTDATA"/container_redis.json > "$newconfig"

	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	output=$(crictl exec --sync "$ctr_id" ls /dev/mynull)
	[[ "$output" == "" ]]
}

@test "privileged ctr device add" {
	# In an user namespace we can only bind mount devices from the host, not mknod
	# https://github.com/opencontainers/runc/blob/master/libcontainer/rootfs_linux.go#L480-L481
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	start_crio
	sandbox_config="$TESTDIR"/sandbox_config.json

	jq '	  .linux.security_context.privileged = true' \
		"$TESTDATA"/sandbox_config.json > "$sandbox_config"
	pod_id=$(crictl runp "$sandbox_config")

	jq '	  .devices = [ {
			host_path: "/dev/null",
			container_path: "/dev/mynull",
			permissions: "rwm"
		} ]
		| .linux.security_context.privileged = true' \
		"$TESTDATA"/container_redis.json > "$newconfig"

	ctr_id=$(crictl create "$pod_id" "$newconfig" "$sandbox_config")
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" ls /dev/mynull)
	[[ "$output" == "" ]]
}

@test "privileged ctr add duplicate device as host" {
	# In an user namespace we can only bind mount devices from the host, not mknod
	# https://github.com/opencontainers/runc/blob/master/libcontainer/rootfs_linux.go#L480-L481
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	start_crio
	sandbox_config="$TESTDIR"/sandbox_config.json

	jq '	  .linux.security_context.privileged = true' \
		"$TESTDATA"/sandbox_config.json > "$sandbox_config"
	pod_id=$(crictl runp "$sandbox_config")

	jq '	  .devices = [ {
			host_path: "/dev/null",
			container_path: "/dev/random",
			permissions: "rwm"
		} ]
		| .linux.security_context.privileged = true
		| del(.linux.security_context.capabilities)' \
		"$TESTDATA"/container_redis.json > "$newconfig"

	# Error is "configured with a device container path that already exists on the host"
	! crictl create "$pod_id" "$newconfig" "$sandbox_config"
}

@test "ctr execsync failure" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id="invalidid"
	! crictl exec --sync "$ctr_id" doesnotexist
}

@test "ctr stop idempotent" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl stop "$ctr_id"
	crictl stop "$ctr_id"
}

@test "ctr caps drop" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .linux.security_context.capabilities = {
			"add_capabilities": [],
			"drop_capabilities": ["mknod", "kill", "sys_chroot", "setuid", "setgid"]
		}' \
		"$TESTDATA"/container_config.json > "$newconfig"

	crictl create "$newconfig" "$TESTDATA"/sandbox_config.json
}

@test "ctr with default list of capabilities from crio.conf" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	output=$(crictl exec --sync "$ctr_id" grep Cap /proc/1/status)

	# This magic value originates from the output of
	# `grep CapEff /proc/self/status`
	#
	# It represents the bitflag of the effective capabilities
	# available to the process.
	[[ "$output" = "" ]]
}

@test "ctr with list of capabilities given by user in crio.conf" {
	CONTAINER_DEFAULT_CAPABILITIES="CHOWN,DAC_OVERRIDE,FSETID,FOWNER,NET_RAW,SETGID,SETUID" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" grep Cap /proc/1/status)
	[[ "$output" = "" ]]
}

@test "ctr with add_inheritable_capabilities has inheritable capabilities" {
	CONTAINER_ADD_INHERITABLE_CAPABILITIES=true start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .linux.security_context.run_as_username = "redis"' \
		"$TESTDATA"/container_redis.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	crictl exec --sync "$ctr_id" grep "CapEff:\s0000000000000000" /proc/1/status
}

@test "ctr create with non-existent command" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .command = ["nonexistent"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]
}

@test "ctr create with non-existent command [tty]" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .command = ["nonexistent"]
		| .tty = true' \
		"$TESTDATA"/container_config.json > "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]
}

@test "ctr correctly setup working directory" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .working_dir = "/thisshouldntexistatall"' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	jq '	  .working_dir = "/etc/passwd"
		| .metadata.name = "container2"' \
		< "$TESTDATA"/container_config.json > "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	[ "$status" -ne 0 ]
	[[ "$output" == *"not a directory"* ]]
}


@test "ctr with low memory configured should not be created" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .linux.resources.memory_limit_in_bytes = 2000' \
		"$TESTDATA"/container_config.json > "$newconfig"
	! crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
}


@test "ctr with absent mount that should be rejected" {
	ABSENT_DIR="$TESTDIR/notthere"
	jq --arg path "$ABSENT_DIR" \
		'  .mounts = [ {
			host_path: $path,
			container_path: $path
		} ]' \
		"$TESTDATA"/container_redis.json > "$TESTDIR/config"

	CONTAINER_ABSENT_MOUNT_SOURCES_TO_REJECT="$ABSENT_DIR" start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	! crictl create "$pod_id" "$TESTDIR/config" "$TESTDATA"/sandbox_config.json
}

@test "ctr stop timeouts should decrease" {
	start_crio
	jq '	  .command'='["/bin/sh", "-c", "trap \"echo hi\" INT; /bin/sleep 6000"]' \
		"$TESTDATA"/container_config.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)
	for i in {150..1}; do
		crictl stop --timeout "$i" "$ctr_id" &
		sleep .1
	done
	crictl stop "$ctr_id"
}
