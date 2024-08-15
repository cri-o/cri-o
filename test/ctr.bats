#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	newconfig="$TESTDIR/config.json"
}

function teardown() {
	cleanup_test
}

# list_all_children lists children of a process recursively
function list_all_children {
	children=$(pgrep -P "$1")
	for i in ${children}; do
		if [ -z "$i" ]; then
			exit
		fi
		echo -n "$i "
		list_all_children "$i"
	done
}

function crictl_rm_preserve_logs {
	ARGS=
	if check_crictl_version 1.30; then
		ARGS=-k
	fi
	crictl rm $ARGS "$1"
}

function check_oci_annotation() {
	# check for OCI annotation in container's config.json
	local ctr_id="$1"
	local key="$2"
	local value="$3"

	config=$(runtime state "$ctr_id" | jq -r .bundle)/config.json

	[ "$(jq -r .annotations.\""$key"\" < "$config")" = "$value" ]
}

# Helper to create two read/write volumes within the test directory,
# where the second volume, or a mount point, rather, will be nested
# within the first one. The helper outputs the path to the test
# volume (mount point) where it was created.
# Note: There is no need to explicitly clean, or unmount if you wish,
# the mounts points this helper creates, as these will be automatically
# cleaned up as part of the test teardown() function run.
function create_test_rro_mounts() {
	# Parent of "--root", keep in sync with test/helpers.bash file.
	directory="$TESTDIR"/test-volume

	mkdir -p "$directory"
	mount -t tmpfs none "$directory"

	mkdir -p "$directory"/test-sub-volume
	mount -t tmpfs none "$directory"/test-sub-volume

	echo "$directory"
}

@test "ctr not found correct error message" {
	start_crio
	run ! crictl inspect "container_not_exist"
}

@test "ctr termination reason Completed" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	wait_until_exit "$ctr_id"

	output=$(crictl inspect --output yaml "$ctr_id")
	[[ "$output" == *"reason: Completed"* ]]
}

@test "ctr termination reason Error" {
	start_crio
	jq '	  .command = ["false"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)

	EXPECTED_EXIT_STATUS=1 wait_until_exit "$ctr_id"

	output=$(crictl inspect --output yaml "$ctr_id")
	[[ "$output" == *"reason: Error"* ]]
}

@test "ulimits" {
	OVERRIDE_OPTIONS="--default-ulimits nofile=42:42 --default-ulimits nproc=1024:2048" start_crio

	jq '	  .command = ["/bin/sh", "-c", "sleep 600"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec --sync "$ctr_id" sh -c "ulimit -n")
	[ "$output" == "42" ]

	output=$(crictl exec --sync "$ctr_id" sh -c "ulimit -u")
	[ "$output" == "1024" ]

	output=$(crictl exec --sync "$ctr_id" sh -c "ulimit -Hu")
	[ "$output" == "2048" ]
}

@test "ctr remove" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
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

@test "ctr pod lifecycle with evented pleg enabled" {
	ENABLE_POD_EVENTS=true start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	output=$(crictl pods --quiet)
	[[ "$output" == "$pod_id" ]]

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	output=$(crictl ps --quiet --state created)
	[[ "$output" == "$ctr_id" ]]
	wait_for_log "Container event CONTAINER_CREATED_EVENT generated"

	crictl start "$ctr_id"
	wait_for_log "Container event CONTAINER_STARTED_EVENT generated"
	output=$(crictl ps --quiet --state running)
	[[ "$output" == "$ctr_id" ]]

	crictl stop "$ctr_id"
	wait_for_log "Container event CONTAINER_STOPPED_EVENT generated"
	output=$(crictl ps --quiet --state exited)
	[[ "$output" == "$ctr_id" ]]

	crictl rm "$ctr_id"
	wait_for_log "Container event CONTAINER_DELETED_EVENT generated"

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

@test "ctr logging" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create a new container.
	jq '	  .command = ["sh", "-c", "echo here is some output && echo and some from stderr >&2"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"
	crictl_rm_preserve_logs "$ctr_id"

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	grep -E "^[^\n]+ stdout F here is some output$" "$logpath"
	grep -E "^[^\n]+ stderr F and some from stderr$" "$logpath"
}

@test "ctr log cleaned up if container create failed" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create a new container.
	jq '	  .command = ["invalid"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	run ! crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json

	# CRI-O should cleanup the log if the container failed to create
	for file in "$DEFAULT_LOG_PATH/$pod_id"/*; do
		[[ "$file" != "$pod_id" ]]
	done
}

@test "ctr journald logging" {
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi
	if ! check_journald; then
		skip "journald logging not supported"
	fi

	CONTAINER_LOG_JOURNALD=true start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create a new container.
	jq '	  .command = ["sh", "-c", "echo here is some output && echo and some from stderr >&2"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"
	crictl rm "$ctr_id"

	# priority of 5 is LOG_NOTICE
	journalctl -p info CONTAINER_ID_FULL="$ctr_id" | grep -F "here is some output"
	# priority of 3 is LOG_ERR
	journalctl -p err CONTAINER_ID_FULL="$ctr_id" | grep -F "and some from stderr"
}

@test "ctr logging [tty=true]" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create a new container.
	jq '	  .command = ["sh", "-c", "echo here is some output && echo and some from stderr >&2"]
		| .tty = true' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"

	output=$(crictl logs "$ctr_id")
	[[ "$output" == *"here is some output"* ]]
}

@test "ctr log max" {
	CONTAINER_LOG_SIZE_MAX=10000 start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create a new container.
	jq '	  .command = ["sh", "-c", "for i in $(seq 250); do echo $i; done"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"
	crictl_rm_preserve_logs "$ctr_id"

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	len=$(wc -l "$logpath" | awk '{print $1}')
	[ "$len" -lt 250 ]
}

@test "ctr log max with default value" {
	# Start crio with default log size max value -1
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create a new container.
	jq '	  .command = ["sh", "-c", "for i in $(seq 250); do echo $i; done"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)

	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"
	crictl_rm_preserve_logs "$ctr_id"

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	# Filter out https://github.com/opencontainers/runc/blob/02120488a4c0fc487d1ed2867e901eeed7ce8ecf/libcontainer/specconv/spec_linux.go#L1017-L1030
	len=$(grep -cv config.json "$logpath")
	[ "$len" -eq 250 ]
}

@test "ctr log max with minimum value" {
	# Start crio with minimum log size max value 8192
	CONTAINER_LOG_SIZE_MAX=8192 start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create a new container.
	jq '	  .command = ["sh", "-c", "for i in $(seq 250); do echo $i; done"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)

	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"
	crictl_rm_preserve_logs "$ctr_id"

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	len=$(wc -l "$logpath" | awk '{print $1}')
	[ "$len" -lt 250 ]
}

@test "ctr partial line logging" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create a new container.
	jq '	  .command = ["sh", "-c", "echo -n hello"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"
	crictl_rm_preserve_logs "$ctr_id"

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	grep -E "^[^\n]+ stdout P hello$" "$logpath"
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

@test "ctr execsync conflicting with conmon flags parsing" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sh -c "echo hello world")
	[ "$output" = "hello world" ]
}

@test "ctr execsync" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" echo HELLO)
	[ "$output" = "HELLO" ]

	run ! crictl -D exec --sync --timeout 3 "$ctr_id" sleep 5
	[[ "$output" == *"command "*" timed out"* ]]
}

@test "ctr execsync should not overwrite initial spec args" {
	start_crio

	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl inspect "$ctr_id")
	[ -n "$output" ]
	echo "$output" | jq -e '.info.runtimeSpec.process.args[2] == "redis-server"'

	crictl exec --sync "$ctr_id" echo Hello

	output=$(crictl inspect "$ctr_id")
	[ -n "$output" ]
	echo "$output" | jq -e '.info.runtimeSpec.process.args[2] == "redis-server"'

	crictl rm -f "$ctr_id"
}

@test "ctr execsync should succeed if container has a terminal" {
	start_crio

	jq ' .tty = true' "$TESTDATA"/container_sleep.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl exec --sync "$ctr_id" /bin/sh -c "[[ -t 1 ]]"
}

@test "ctr execsync should cap output" {
	start_crio

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	[[ $(crictl exec --sync "$ctr_id" /bin/sh -c "for i in $(seq 1 50000000); do echo -n 'a'; done" | wc -c) -le 16777216 ]]
}

@test "ctr exec{,sync} should be cancelled when container is stopped" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	crictl exec --sync "$ctr_id" /bin/bash -c 'while true; do echo XXXXXXXXXXXXXXXXXXXXXXXX; done' &
	pid1=$!
	crictl exec "$ctr_id" /bin/bash -c 'while true; do echo XXXXXXXXXXXXXXXXXXXXXXXX; done' || true &
	pid2=$!

	sleep 1s

	crictl stop "$ctr_id"
	wait "$pid1" "$pid2"
}

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
	[[ "$output" == *"/dev/mynull"* ]]
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
	[[ "$output" == *"/dev/mynull"* ]]
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
	run ! crictl create "$pod_id" "$newconfig" "$sandbox_config"
}

@test "ctr hostname env" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" == *"HOSTNAME"* ]]
}

@test "ctr execsync failure" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	run ! crictl exec --sync "$ctr_id" doesnotexist
}

@test "ctr execsync exit code" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	run ! crictl exec --sync "$ctr_id" false
}

@test "ctr execsync std{out,err}" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" echo hello0 stdout)
	[[ "$output" == *"hello0 stdout"* ]]

	output=$(crictl exec --sync "$ctr_id" /bin/sh -c "echo hello0 stderr >&2")
	[[ "$output" == *"hello0 stderr"* ]]
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
	[[ "$output" =~ 00000000002005fb ]]
}

@test "ctr with list of capabilities given by user in crio.conf" {
	CONTAINER_DEFAULT_CAPABILITIES="CHOWN,DAC_OVERRIDE,FSETID,FOWNER,NET_RAW,SETGID,SETUID" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" grep Cap /proc/1/status)
	[[ "$output" =~ 00000000002020db ]]
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

@test "ctr /etc/resolv.conf rw/ro mode" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq '	  .command = ["sh", "-c", "echo test >> /etc/resolv.conf"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"

	jq '	  .command = ["sh", "-c", "echo test >> /etc/resolv.conf"]
		| .linux.security_context.readonly_rootfs = true
		| .metadata.name = "test-resolv-ro"' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	EXPECTED_EXIT_STATUS=1 wait_until_exit "$ctr_id"
}

@test "ctr create with non-existent command" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .command = ["nonexistent"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	run ! crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	[[ "$output" == *"not found"* ]]
}

@test "ctr create with non-existent command [tty]" {
	if is_using_crun; then
		skip "not supported by crun: https://github.com/containers/crun/issues/1524"
	fi

	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .command = ["nonexistent"]
		| .tty = true' \
		"$TESTDATA"/container_config.json > "$newconfig"
	run ! crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	[[ "$output" == *"not found"* ]]
}

@test "ctr update resources" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	set_swap_fields_given_cgroup_version

	output=$(crictl exec --sync "$ctr_id" sh -c "cat $CGROUP_MEM_FILE")
	[[ "$output" == *"209715200"* ]]

	# we can only rely on these files being here if cgroup memory swap is enabled
	# otherwise this test fails
	if test -r "$CGROUP_MEM_SWAP_FILE"; then
		output=$(crictl exec --sync "$ctr_id" sh -c "cat $CGROUP_MEM_SWAP_FILE")
		[ "$output" -eq "209715200" ]
	fi

	if is_cgroup_v2; then
		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu.max")
		[[ "$output" == *"20000 10000"* ]]

		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu.weight")
		# 512 shares are converted to cpu.weight 20
		[[ "$output" == *"20"* ]]
	else
		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.shares")
		[[ "$output" == *"512"* ]]

		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.cfs_period_us")
		[[ "$output" == *"10000"* ]]

		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.cfs_quota_us")
		[[ "$output" == *"20000"* ]]
	fi

	crictl update --memory 524288000 --cpu-period 20000 --cpu-quota 10000 --cpu-share 256 "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sh -c "cat $CGROUP_MEM_FILE")
	[[ "$output" == *"524288000"* ]]

	if test -r "$CGROUP_MEM_SWAP_FILE"; then
		output=$(crictl exec --sync "$ctr_id" sh -c "cat $CGROUP_MEM_SWAP_FILE")
		[ "$output" -eq "524288000" ]
	fi

	if is_cgroup_v2; then
		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu.max")
		[[ "$output" == *"10000 20000"* ]]

		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu.weight")
		# 256 shares are converted to cpu.weight 10
		[[ "$output" == *"10"* ]]
	else
		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.shares")
		[[ "$output" == *"256"* ]]

		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.cfs_period_us")
		[[ "$output" == *"20000"* ]]

		output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.cfs_quota_us")
		[[ "$output" == *"10000"* ]]
	fi
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
	run ! crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	[[ "$output" == *"not a directory"* ]]
}

@test "ctr execsync conflicting with conmon env" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# XXX: this relies on PATH being the first element in envs[]
	jq '	  .envs[0].value += ":/acustompathinpath"' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec "$ctr_id" env)
	[[ "$output" == *"acustompathinpath"* ]]

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" == *"acustompathinpath"* ]]
}

@test "ctr resources" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpuset/cpuset.cpus || cat /sys/fs/cgroup/cpuset.cpus")
	[[ "$output" == *"0"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpuset/cpuset.mems || cat /sys/fs/cgroup/cpuset.mems")
	[[ "$output" == *"0"* ]]
}

@test "ctr with non-root user has no effective capabilities" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .linux.security_context.run_as_username = "redis"' \
		"$TESTDATA"/container_redis.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	crictl exec --sync "$ctr_id" grep "CapEff:\s0000000000000000" /proc/1/status
}

@test "ctr has gid in supplemental groups" {
	start_crio

	jq '	  .linux.security_context.run_as_user.value = 1000
		|     .linux.security_context.run_as_group.value = 1000' \
		"$TESTDATA"/container_redis.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)

	crictl exec --sync "$ctr_id" grep Groups:.1000 /proc/1/status
}

@test "ctr has gid in supplemental groups with Merge policy" {
	start_crio
	jq '	  .linux.security_context.supplemental_groups_policy = 0' \
		"$TESTDATA"/sandbox_config.json > "$newconfig"

	pod_id=$(crictl runp "$newconfig")
	jq '	  .image.image = "quay.io/crio/fedora-crio-ci:latest"
	    |     .linux.security_context.supplemental_groups = [10]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/container_sleep_modified.json

	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container_sleep_modified.json "$newconfig")

	crictl exec --sync "$ctr_id" id | grep -q "10"
}

@test "ctr has only specified gid in supplemental groups with Strict policy" {
	start_crio
	jq '	  .linux.security_context.supplemental_groups_policy = 1' \
		"$TESTDATA"/sandbox_config.json > "$newconfig"

	pod_id=$(crictl runp "$newconfig")
	jq '	  .image.image = "quay.io/crio/fedora-crio-ci:latest"
	    |     .linux.security_context.supplemental_groups = [10]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/container_sleep_modified.json

	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container_sleep_modified.json "$newconfig")

	# Ensure 10 should not be present in supplemental groups.
	crictl exec --sync "$ctr_id" id | grep -vq "10"
}

@test "ctr with low memory configured should not be created" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .linux.resources.memory_limit_in_bytes = 2000' \
		"$TESTDATA"/container_config.json > "$newconfig"
	run ! crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
}

@test "privileged ctr -- check for rw mounts" {
	# Can't run privileged container in userns
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	start_crio

	sandbox_config="$TESTDIR"/sandbox_config.json

	jq '	  .linux.security_context.privileged = true' \
		"$TESTDATA"/sandbox_config.json > "$sandbox_config"
	pod_id=$(crictl runp "$sandbox_config")

	jq '	  .linux.security_context.privileged = true' \
		"$TESTDATA"/container_redis.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$sandbox_config")
	crictl start "$ctr_id"

	output=$(crictl inspect "$ctr_id")
	[ -n "$output" ]
	echo "$output" | jq -e ".info.privileged == true"

	output=$(crictl exec "$ctr_id" grep rw\, /proc/mounts)
	if is_cgroup_v2; then
		[[ "$output" == *" /sys/fs/cgroup cgroup2 "* ]]
	else
		[[ "$output" == *" /sys/fs/cgroup tmpfs "* ]]
	fi
}

@test "annotations passed through" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	crictl inspectp "$pod_id" | grep '"owner": "hmeng"'
	crictl inspectp "$pod_id" | grep '"security.alpha.kubernetes.io/seccomp/pod": "unconfined"'

	# sandbox annotations passed through to container OCI config
	ctr_id=$(crictl run "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	check_oci_annotation "$ctr_id" "com.example.test" "sandbox annotation"
}

@test "ctr with default_env set in configuration" {
	CONTAINER_DEFAULT_ENV="NSS_SDB_USE_CACHE=no" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec --sync "$ctr_id" env)
	[[ "$output" == *"NSS_SDB_USE_CACHE=no"* ]]
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
	run ! crictl create "$pod_id" "$TESTDIR/config" "$TESTDATA"/sandbox_config.json
}

@test "ctr that mounts container storage as shared should keep shared" {
	# parent of `--root`, keep in sync with test/helpers.bash
	PARENT_DIR="$TESTDIR"
	CTR_DIR="/host"
	jq --arg path "$PARENT_DIR" --arg ctr_dir "$CTR_DIR" \
		'  .mounts = [ {
			host_path: $path,
			container_path: $ctr_dir,
			propagation: 2
		} ]' \
		"$TESTDATA"/container_redis.json > "$TESTDIR/config"

	start_crio

	ctr_id=$(crictl run "$TESTDIR/config" "$TESTDATA"/sandbox_config.json)
	crictl exec --sync "$ctr_id" findmnt -no TARGET,PROPAGATION "$CTR_DIR" | grep shared
}

@test "ctr that mounts container storage as private should not be private" {
	# parent of `--root`, keep in sync with test/helpers.bash
	PARENT_DIR="$TESTDIR"
	CTR_DIR="/host"
	jq --arg path "$PARENT_DIR" --arg ctr_dir "$CTR_DIR" \
		'  .mounts = [ {
			host_path: $path,
			container_path: $ctr_dir,
			propagation: 1
		} ]' \
		"$TESTDATA"/container_redis.json > "$TESTDIR/config"

	start_crio

	ctr_id=$(crictl run "$TESTDIR/config" "$TESTDATA"/sandbox_config.json)
	crictl exec --sync "$ctr_id" findmnt -no TARGET,PROPAGATION "$CTR_DIR" | grep -v private
}

@test "ctr that mounts container storage as read-only option but not recursively" {
	# When SELinux is enabled and set to Enforcing, then the read-only
	# mounts within a container will stop sub-mounts access in a read-write
	# manner, and this test will then fail, thus it's best to disable it.
	# Note: This is not a problem on a systems without SELinux.
	if is_selinux_enforcing; then
		skip "SELinux is set to Enforcing"
	fi

	# See https://www.shellcheck.net/wiki/SC2154 for more details.
	declare stderr

	PARENT_DIR="$(create_test_rro_mounts)"
	CTR_DIR="/host"

	jq --arg path "$PARENT_DIR" --arg ctr_dir "$CTR_DIR" \
		'  .mounts = [ {
			host_path: $path,
			container_path: $ctr_dir,
			readonly: true,
			propagation: 0
		} ]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/config

	start_crio

	ctr_id=$(crictl run "$TESTDIR"/config "$TESTDATA"/sandbox_config.json)

	run ! --separate-stderr crictl exec --sync "$ctr_id" touch /host/test
	[[ "$stderr" == *"Read-only file system"* ]]

	crictl exec --sync "$ctr_id" touch /host/test-sub-volume/test
}

@test "ctr that mounts container storage as recursively read-only" {
	requires_kernel "5.12"

	# Check for the minimum cri-tools version that supports RRO mounts.
	requires_crictl "1.30"

	# See https://www.shellcheck.net/wiki/SC2154 for more details.
	declare stderr

	PARENT_DIR="$(create_test_rro_mounts)"
	CTR_DIR="/host"

	jq --arg path "$PARENT_DIR" --arg ctr_dir "$CTR_DIR" \
		'  .mounts = [ {
			host_path: $path,
			container_path: $ctr_dir,
			readonly: true,
			recursive_read_only: true,
			propagation: 0
		} ]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/config

	start_crio

	ctr_id=$(crictl run "$TESTDIR"/config "$TESTDATA"/sandbox_config.json)

	run ! --separate-stderr crictl exec --sync "$ctr_id" touch /host/test
	[[ "$stderr" == *"Read-only file system"* ]]

	run ! --separate-stderr crictl exec --sync "$ctr_id" touch /host/test-sub-volume/test
	[[ "$stderr" == *"Read-only file system"* ]]
}

@test "ctr that fails to mount container storage as recursively read-only without readonly option" {
	requires_kernel "5.12"

	# Check for the minimum cri-tools version that supports RRO mounts.
	requires_crictl "1.30"

	# See https://www.shellcheck.net/wiki/SC2154 for more details.
	declare stderr

	# Parent of "--root", keep in sync with test/helpers.bash file.
	PARENT_DIR="$TESTDIR"
	CTR_DIR="/host"

	jq --arg path "$PARENT_DIR" --arg ctr_dir "$CTR_DIR" \
		'  .mounts = [ {
			host_path: $path,
			container_path: $ctr_dir,
			readonly: false,
			recursive_read_only: true,
		} ]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/config

	start_crio

	run ! --separate-stderr crictl run "$TESTDIR"/config "$TESTDATA"/sandbox_config.json
	[[ "$stderr" == *"recursive read-only mount conflicts with read-write mount"* ]]
}

@test "ctr that fails to mount container storage as recursively read-only without private propagation" {
	requires_kernel "5.12"

	# Check for the minimum cri-tools version that supports RRO mounts.
	requires_crictl "1.30"

	# See https://www.shellcheck.net/wiki/SC2154 for more details.
	declare stderr

	# Parent of "--root", keep in sync with test/helpers.bash file.
	PARENT_DIR="$TESTDIR"
	CTR_DIR="/host"

	jq --arg path "$PARENT_DIR" --arg ctr_dir "$CTR_DIR" \
		'  .mounts = [ {
			host_path: $path,
			container_path: $ctr_dir,
			readonly: true,
			recursive_read_only: true,
			propagation: 2
		} ]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/config

	start_crio

	run ! --separate-stderr crictl run "$TESTDIR"/config "$TESTDATA"/sandbox_config.json
	[[ "$stderr" == *"recursive read-only mount requires private propagation"* ]]
}

@test "ctr has containerenv" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	crictl exec --sync "$ctr_id" sh -c "stat /run/.containerenv"
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

@test "ctr with node level pid namespace should not leak children" {
	if [[ "$RUNTIME_TYPE" == "vm" ]]; then
		skip "not applicable to vm runtime type"
	fi
	if [[ "$TEST_USERNS" == "1" ]]; then
		skip "test fails in a user namespace"
	fi
	newsandbox="$TESTDIR/sandbox.json"
	start_crio

	jq '	  .linux.security_context.namespace_options.pid = 2' \
		"$TESTDATA"/sandbox_config.json > "$newsandbox"

	jq '	  .linux.security_context.namespace_options.pid = 2
		| .command = ["/bin/sh", "-c", "sleep 1m& exec sleep 2m"]' \
		"$TESTDATA"/container_config.json > "$newconfig"

	ctr_id=$(crictl run "$newconfig" "$newsandbox")
	processes=$(list_all_children "$(pidof conmon)")

	pid=$(runtime list -f json | jq .[].pid)
	[[ "$pid" -gt 0 ]]
	kill -9 "$pid"

	EXPECTED_EXIT_STATUS=137 wait_until_exit "$ctr_id"

	# make sure crio syncs state
	for process in ${processes}; do
		# Ignore Z state (zombies) as the process has just been killed and reparented. Systemd will get to it.
		# `pgrep` doesn't have a good mechanism for ignoring Z state, but including all others, so:
		# shellcheck disable=SC2143
		[ -z "$(ps -p "$process" o pid=,stat= | grep -v ' Z')" ]
	done
}

@test "ctr HOME env newline invalid" {
	start_crio
	jq ' .envs = [{"key": "HOME=", "value": "/root:/sbin/nologin\\ntest::0:0::/:/bin/bash"}]' \
		"$TESTDATA"/container_config.json > "$newconfig"

	run ! crictl run "$newconfig" "$TESTDATA"/sandbox_config.json
}

@test "ctr log linking" {
	if [[ $RUNTIME_TYPE == vm ]]; then
		skip "not applicable to vm runtime type"
	fi
	create_runtime_with_allowed_annotation logs io.kubernetes.cri-o.LinkLogs
	start_crio

	# Create directories created by the kubelet needed for log linking to work
	pod_uid=$(head -c 32 /proc/sys/kernel/random/uuid)
	pod_name=$(jq -r '.metadata.name' "$TESTDATA/sandbox_config.json")
	pod_namespace=$(jq -r '.metadata.namespace' "$TESTDATA/sandbox_config.json")
	pod_log_dir="/var/log/pods/${pod_namespace}_${pod_name}_${pod_uid}"
	mkdir -p "$pod_log_dir"
	pod_empty_dir_volume_path="/var/lib/kubelet/pods/$pod_uid/volumes/kubernetes.io~empty-dir/logging-volume"
	mkdir -p "$pod_empty_dir_volume_path"
	ctr_path="/mnt/logging-volume"

	ctr_name=$(jq -r '.metadata.name' "$TESTDATA/container_config.json")
	ctr_attempt=$(jq -r '.metadata.attempt' "$TESTDATA/container_config.json")

	# Add annotation for log linking in the pod
	jq --arg pod_log_dir "$pod_log_dir" --arg pod_uid "$pod_uid" '.annotations["io.kubernetes.cri-o.LinkLogs"] = "logging-volume"
	| .log_directory = $pod_log_dir | .metadata.uid = $pod_uid' \
		"$TESTDATA/sandbox_config.json" > "$TESTDIR/sandbox_config.json"
	pod_id=$(crictl runp "$TESTDIR"/sandbox_config.json)

	# Touch the log file
	mkdir -p "$pod_log_dir/$ctr_name"
	touch "$pod_log_dir/$ctr_name/$ctr_attempt.log"

	# Create a new container
	jq --arg host_path "$pod_empty_dir_volume_path" --arg ctr_path "$ctr_path" --arg log_path "$ctr_name/$ctr_attempt.log" \
		'	  .command = ["sh", "-c", "echo Hello log linking && sleep 1000"]
		| .log_path = $log_path
		| .mounts = [ {
				host_path: $host_path,
				container_path: $ctr_path
			} ]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDIR/sandbox_config.json")

	# Check that the log is linked
	ctr_log_path="$pod_log_dir/$ctr_name/$ctr_attempt.log"
	[ -f "$ctr_log_path" ]
	mounted_log_path="$pod_empty_dir_volume_path/$ctr_name/$ctr_attempt.log"
	[ -f "$mounted_log_path" ]
	linked_log_path="$pod_empty_dir_volume_path/$ctr_id.log"
	[ -f "$linked_log_path" ]

	crictl start "$ctr_id"

	# Check expected file contents
	grep -E "Hello log linking" "$mounted_log_path"
	grep -E "Hello log linking" "$ctr_log_path"
	grep -E "Hello log linking" "$linked_log_path"

	crictl exec --sync "$ctr_id" grep -E "Hello log linking" "$ctr_path"/"$ctr_id.log"

	# Check linked logs were cleaned up
	crictl rmp -fa
	[ ! -f "$mounted_log_path" ]
	[ ! -f "$linked_log_path" ]
}

@test "ctr stop loop kill retry attempts" {
	FAKE_RUNTIME_BINARY_PATH="$TESTDIR"/fake
	FAKE_RUNTIME_ATTEMPTS_LOG="$TESTDIR"/fake.log

	# Both values should be adjusted to match the current
	# exponential backoff configuration of the container
	# stop loop retry logic.
	FAKE_RUNTIME_ATTEMPTS_LIMIT=10
	FAKE_RUNTIME_ATTEMPTS_TIME_DURATION=30 # Seconds.

	cat << EOF > "$FAKE_RUNTIME_BINARY_PATH"
#!/usr/bin/env bash
set -eo pipefail
[[ \$* == *kill* ]] && {
  attempts=\$(wc -l $FAKE_RUNTIME_ATTEMPTS_LOG || echo 0) ;
  date +'%s' >> $FAKE_RUNTIME_ATTEMPTS_LOG ;
  (( \${attempts%% *} > $FAKE_RUNTIME_ATTEMPTS_LIMIT )) || exit 0 ;
}
exec $RUNTIME_BINARY_PATH "\$@"
EOF

	cat << EOF > "$CRIO_CONFIG_DIR"/99-fake-runtime.conf
[crio.runtime]
default_runtime = "fake"
[crio.runtime.runtimes.fake]
runtime_path = "$FAKE_RUNTIME_BINARY_PATH"
EOF
	chmod 755 "$FAKE_RUNTIME_BINARY_PATH"

	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	crictl start "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	grep -q "Stopping container ${ctr_id} with stop signal timed out." "$CRIO_LOG"

	readarray -t attempts < "$FAKE_RUNTIME_ATTEMPTS_LOG"

	if ((${#attempts[@]} < FAKE_RUNTIME_ATTEMPTS_LIMIT)); then
		echo "Container stop loop should have at least ${FAKE_RUNTIME_ATTEMPTS_LIMIT} kill attempts" >&3
		return 1
	fi

	# The exponential backoff is not working if there are too many retry attempts.
	if ((${#attempts[@]} > 100)); then
		echo "Container stop loop has too many kill attempts" >&3
		return 1
	fi

	# The test should run long enough to retry over 10 times, where the first
	# and the last timestamp of when the kill command was invoked will be
	# about a minute apart. As such, 30 seconds should be the minimum.
	if ((${attempts[${#attempts[@]} - 1]} - attempts[0] < FAKE_RUNTIME_ATTEMPTS_TIME_DURATION)); then
		echo "Container stop loop kill retry attempts should be at least 30 seconds apart" >&3
		return 1
	fi

	run ! crictl inspect "$ctr_id"
}

@test "ctr multiple stop calls" {
	start_crio

	# Create a container with a long-running command to simulate a scenario where
	# a container takes a while to stop gracefully.
	jq '.command = ["/bin/sh", "-c", "sleep 600"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl run "$newconfig" "$TESTDATA"/sandbox_config.json)

	# Issue the first crictl stop command with a long timeout.
	crictl stop --timeout 3600 "$ctr_id" &
	sleep 5 # Ensure the first stop command has time to start.

	# Attempt to issue another crictl stop command while the first one is still active.
	crictl stop --timeout 0 "$ctr_id" &> /dev/null

	# Verify that the container has either stopped or exited.
	final_state=$(crictl inspect "$ctr_id" | grep -Po '(?<="state": ")[^"]*')
	if [ "$final_state" != "CONTAINER_STOPPED" ] && [ "$final_state" != "CONTAINER_EXITED" ]; then
		echo "Test failed: Container did not stop or exit as expected."
		exit 1
	fi
}
