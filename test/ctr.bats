#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	newconfig="$TESTDIR/config.json"
}

function teardown() {
	cleanup_test
}

function wait_until_exit() {
	ctr_id=$1
	# Wait for container to exit
	attempt=0
	while [ $attempt -le 100 ]; do
		attempt=$((attempt + 1))
		output=$(crictl inspect -o table "$ctr_id")
		if [[ "$output" == *"State: CONTAINER_EXITED"* ]]; then
			[[ "$output" == *"Exit Code: ${EXPECTED_EXIT_STATUS:-0}"* ]]
			return 0
		fi
		sleep 1
	done
	return 1
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

function check_oci_annotation() {
	# check for OCI annotation in container's config.json
	local ctr_id="$1"
	local key="$2"
	local value="$3"

	config=$(runtime state "$ctr_id" | jq -r .bundle)/config.json

	[ "$(jq -r .annotations.\""$key"\" < "$config")" = "$value" ]
}

@test "ctr not found correct error message" {
	start_crio
	! crictl inspect "container_not_exist"
}

@test "ctr termination reason Completed" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"

	output=$(crictl inspect --output yaml "$ctr_id")
	[[ "$output" == *"reason: Completed"* ]]
}

@test "ctr termination reason Error" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq '	  .command = ["false"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)

	crictl start "$ctr_id"
	EXPECTED_EXIT_STATUS=1 wait_until_exit "$ctr_id"

	output=$(crictl inspect --output yaml "$ctr_id")
	[[ "$output" == *"reason: Error"* ]]
}

@test "ulimits" {
	OVERRIDE_OPTIONS="--default-ulimits nofile=42:42 --default-ulimits nproc=1024:2048" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .command = ["/bin/sh", "-c", "sleep 600"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sh -c "ulimit -n")
	[ "$output" == "42" ]

	output=$(crictl exec --sync "$ctr_id" sh -c "ulimit -p")
	[ "$output" == "1024" ]

	output=$(crictl exec --sync "$ctr_id" sh -c "ulimit -Hp")
	[ "$output" == "2048" ]
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

@test "ctr logging" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create a new container.
	jq '	  .command = ["sh", "-c", "echo here is some output && echo and some from stderr >&2"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	wait_until_exit "$ctr_id"
	crictl rm "$ctr_id"

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
	! crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json

	# CRI-O should cleanup the log if the container failed to create
	for file in "$DEFAULT_LOG_PATH/$pod_id"/*; do
		[[ "$file" != "$pod_id" ]]
	done
}

@test "ctr journald logging" {
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
	crictl rm "$ctr_id"

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
	crictl rm "$ctr_id"

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	len=$(wc -l "$logpath" | awk '{print $1}')
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
	crictl rm "$ctr_id"

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
	crictl rm "$ctr_id"

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

	run crictl -D exec --sync --timeout 3 "$ctr_id" sleep 5
	echo "$output"
	[[ "$output" == *"command "*" timed out"* ]]
	[ "$status" -ne 0 ]
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
	! crictl create "$pod_id" "$newconfig" "$sandbox_config"
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

	! crictl exec --sync "$ctr_id" doesnotexist
}

@test "ctr execsync exit code" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	! crictl exec --sync "$ctr_id" false
}

@test "ctr execsync std{out,err}" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" echo hello0 stdout)
	[[ "$output" == *"hello0 stdout"* ]]

	jq '	  .image.image = "quay.io/crio/stderr-test"
		| .command = ["/bin/sleep", "600"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" stderr)
	[[ "$output" == *"this goes to stderr"* ]]
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

@test "ctr oom" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .image.image = "quay.io/crio/oom"
		| .linux.resources.memory_limit_in_bytes = 25165824
		| .command = ["/oom"]' \
		"$TESTDATA"/container_config.json > "$newconfig"
	ctr_id=$(crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# Wait for container to OOM
	attempt=0
	while [ $attempt -le 100 ]; do
		attempt=$((attempt + 1))
		output=$(crictl inspect --output yaml "$ctr_id")
		if [[ "$output" == *"OOMKilled"* ]]; then
			break
		fi
		sleep 10
	done
	[[ "$output" == *"OOMKilled"* ]]
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
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	[ "$status" -ne 0 ]
	[[ "$output" == *"not found"* ]]
}

@test "ctr create with non-existent command [tty]" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .command = ["nonexistent"]
		| .tty = true' \
		"$TESTDATA"/container_config.json > "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	[ "$status" -ne 0 ]
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
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	[ "$status" -ne 0 ]
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
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

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

@test "ctr with low memory configured should not be created" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .linux.resources.memory_limit_in_bytes = 2000' \
		"$TESTDATA"/container_config.json > "$newconfig"
	! crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
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
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
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
	! crictl create "$pod_id" "$TESTDIR/config" "$TESTDATA"/sandbox_config.json
}

@test "ctr has containerenv" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

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
	if [[ -n "$TEST_USERNS" ]]; then
		skip "test fails in a user namespace"
	fi
	newsandbox="$TESTDIR/sandbox.json"
	start_crio

	jq '	  .linux.security_context.namespace_options.pid = 2' \
		"$TESTDATA"/sandbox_config.json > "$newsandbox"

	jq '	  .image.image = "quay.io/crio/redis:alpine"
		| .linux.security_context.namespace_options.pid = 2
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
		# shellcheck disable=SC2009
		! ps -p "$process" o pid=,stat= | grep -v 'Z'
	done
}
