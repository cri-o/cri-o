#!/usr/bin/env bats

load helpers

function setup() {
	newconfig=$(mktemp --tmpdir crio-config.XXXXXX.json)
	setup_test
}

function teardown() {
	rm -f "$newconfig"
	cleanup_test
}

@test "ctr not found correct error message" {
	start_crio
	run crictl inspect "container_not_exist"
	echo "$output"
	[ "$status" -eq 1 ]

	stop_crio
}

@test "ctr termination reason Completed" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
	run crictl inspect --output yaml "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "reason: Completed" ]]
}

@test "ctr termination reason Error" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	errorconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["command"] = ["false"]; json.dump(obj, sys.stdout)')
	echo "$errorconfig" > "$TESTDIR"/container_config_error.json
	run crictl create "$pod_id" "$TESTDIR"/container_config_error.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	EXPECTED_EXIT_STATUS=1 wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
	run crictl inspect --output yaml "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "reason: Error" ]]
}

@test "ulimits" {
	ULIMITS="--default-ulimits nofile=42:42 --default-ulimits nproc=1024:2048" start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	ulimits=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin); obj["command"] = ["/bin/sh", "-c", "sleep 600"]; json.dump(obj, sys.stdout)')
	echo "$ulimits" > "$TESTDIR"/container_config_ulimits.json
	run crictl create "$pod_id" "$TESTDIR"/container_config_ulimits.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" sh -c "ulimit -n"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "42" ]]
	run crictl exec --sync "$ctr_id" sh -c "ulimit -p"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "1024" ]]

	run crictl exec --sync "$ctr_id" sh -c "ulimit -Hp"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "2048" ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "additional devices support" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	DEVICES="--additional-devices /dev/null:/dev/qifoo:rwm" start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" sh -c "ls /dev/qifoo"
	echo $output
	[ "$status" -eq 0 ]
	[ "$output" == "/dev/qifoo" ]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "additional devices permissions" {
	# We need a ubiquitously configured device that isn't in the
	# OCI spec default set.
	local readonly device="/dev/loop-control"
	local readonly timeout=30

	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	if ! test -r $device ; then
		skip "$device not readable"
	fi

	if ! test -w $device ; then
		skip "$device not writeable"
	fi

	DEVICES="--additional-devices ${device}:${device}:w" start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

        # Ensure the device is there.
	run crictl exec --timeout=$timeout --sync "$ctr_id" ls $device
	echo $output
	[ "$status" -eq 0 ]
	[[ "$output" == "$device" ]]

	# Dump the deviced cgroup configuration for debugging.
	run crictl exec --timeout=$timeout --sync "$ctr_id" cat /sys/fs/cgroup/devices/devices.list
	echo $output
	[[ "$output" =~ "c 10:237 w" ]]

        # Opening the device in read mode should fail because the device
        # cgroup access only allows writes.
	run crictl exec --timeout=$timeout --sync "$ctr_id" dd if=$device of=/dev/null count=1
	echo $output
	[[ "$output" =~ "Operation not permitted" ]]

        # The write should be allowed by the devices cgroup policy, so we
        # should see an EINVAL from the device when the device fails it.
	run crictl exec --timeout=$timeout --sync "$ctr_id" dd if=/dev/zero of=$device count=1
	echo $output
	[[ "$output" =~ "Invalid argument" ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}


@test "ctr remove" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rm -f "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr lifecycle" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl pods --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$pod_id" ]]
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl ps --quiet --state created
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr_id" ]]
	run crictl inspect "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl inspect "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl ps --quiet --state running
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr_id" ]]
	run crictl stop "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl inspect "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl ps --quiet --state exited
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr_id" ]]
	run crictl rm "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl ps --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl pods --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$pod_id" ]]
	run crictl ps --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl pods --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
}


@test "ctr logging" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	# Create a new container.
	cp "$TESTDATA"/container_config_logging.json "$newconfig"
	sed -i 's|"%shellcommand%"|"echo here is some output \&\& echo and some from stderr >\&2"|' "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
	run crictl rm "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	grep -E "^[^\n]+ stdout F here is some output$" "$logpath"
	grep -E "^[^\n]+ stderr F and some from stderr$" "$logpath"

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr journald logging" {
	# ensure we have journald logging capability
	enabled=$(check_journald)
	if [[ "$enabled" -ne 0 ]]; then
		skip "journald not enabled"
	fi

	start_crio_journald
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	stdout="here is some output"
	stderr="here is some error"

	# Create a new container.
	cp "$TESTDATA"/container_config_logging.json "$newconfig"
	sed -i 's|"%shellcommand%"|"echo '"$stdout"' \&\& echo '"$stderr"' >\&2"|' "$newconfig"
	cat "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
	run crictl rm "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	# priority of 5 is LOG_NOTICE
	journalctl -t conmon -p info CONTAINER_ID_FULL="$ctr_id" | grep -E "$stdout"
	# priority of 3 is LOG_ERR
	journalctl -t conmon -p err CONTAINER_ID_FULL="$ctr_id" | grep -E "$stderr"

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr logging [tty=true]" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	# Create a new container.
	cp "$TESTDATA"/container_config_logging.json "$newconfig"
	sed -i 's|"%shellcommand%"|"echo here is some output"|' "$newconfig"
	sed -i 's|"tty": false,|"tty": true,|' "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	run crictl logs "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "here is some output" ]]

	run crictl rm "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr log max" {
	CONTAINER_LOG_SIZE_MAX=10000 start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	# Create a new container.
	cp "$TESTDATA"/container_config_logging.json "$newconfig"
	sed -i 's|"%shellcommand%"|"for i in $(seq 250); do echo $i; done"|' "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
	run crictl rm "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	len=$(wc -l "$logpath" | awk '{print $1}')
	[ $len -lt 250 ]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr log max with default value" {
	# Start crio with default log size max value -1
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	# Create a new container.
	cp "$TESTDATA"/container_config_logging.json "$newconfig"
	sed -i 's|"%shellcommand%"|"for i in $(seq 250); do echo $i; done"|' "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
	run crictl rm "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	len=$(wc -l "$logpath" | awk '{print $1}')
	[ $len -eq 250 ]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr log max with minimum value" {
	# Start crio with minimum log size max value 8192
	CONTAINER_LOG_SIZE_MAX=8192 start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	# Create a new container.
	cp "$TESTDATA"/container_config_logging.json "$newconfig"
	sed -i 's|"%shellcommand%"|"for i in $(seq 250); do echo $i; done"|' "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
	run crictl rm "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	len=$(wc -l "$logpath" | awk '{print $1}')
	[ $len -lt 250 ]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr partial line logging" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	# Create a new container.
	cp "$TESTDATA"/container_config_logging.json "$newconfig"
	sed -i 's|"%shellcommand%"|"echo -n hello"|' "$newconfig"
	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
	run crictl rm "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	# Check that the output is what we expect.
	logpath="$DEFAULT_LOG_PATH/$pod_id/$ctr_id.log"
	[ -f "$logpath" ]
	echo "$logpath :: $(cat "$logpath")"
	grep -E "^[^\n]+ stdout P hello$" "$logpath"

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

# regression test for #127
@test "ctrs status for a pod" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl ps --quiet --state created
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" == "$ctr_id" ]]

	printf '%s\n' "$output" | while IFS= read -r id
	do
		run crictl inspect "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
}

@test "ctr list filtering" {
	# start 3 redis sandbox
	# pod1 ctr1 create & start
	# pod2 ctr2 create
	# pod3 ctr3 create & start & stop
	start_crio
	run crictl runp "$TESTDATA"/sandbox1_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod1_id="$output"
	run crictl create "$pod1_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox1_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr1_id="$output"
	run crictl start "$ctr1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl runp "$TESTDATA"/sandbox2_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod2_id="$output"
	run crictl create "$pod2_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox2_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr2_id="$output"
	run crictl runp "$TESTDATA"/sandbox3_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod3_id="$output"
	run crictl create "$pod3_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox3_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr3_id="$output"
	run crictl start "$ctr3_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stop "$ctr3_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl ps --id "$ctr1_id" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr1_id" ]]
	run crictl ps --id "${ctr1_id:0:4}" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr1_id" ]]
	run crictl ps --id "$ctr2_id" --pod "$pod2_id" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr2_id" ]]
	run crictl ps --id "$ctr2_id" --pod "$pod3_id" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
	run crictl ps --state created --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr2_id" ]]
	run crictl ps --state running --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr1_id" ]]
	run crictl ps --state exited --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr3_id" ]]
	run crictl ps --pod "$pod1_id" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr1_id" ]]
	run crictl ps --pod "$pod2_id" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr2_id" ]]
	run crictl ps --pod "$pod3_id" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr3_id" ]]
	run crictl stopp "$pod1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod2_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod2_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stopp "$pod3_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod3_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr list label filtering" {
	# start a pod with 3 containers
	# ctr1 with labels: group=test container=redis version=v1.0.0
	# ctr2 with labels: group=test container=redis version=v1.0.0
	# ctr3 with labels: group=test container=redis version=v1.1.0
	start_crio

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	ctrconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["metadata"]["name"] = "ctr1";obj["labels"]["group"] = "test";obj["labels"]["name"] = "ctr1";obj["labels"]["version"] = "v1.0.0"; json.dump(obj, sys.stdout)')
	echo "$ctrconfig" > "$TESTDATA"/labeled_container_redis.json
	run crictl create "$pod_id" "$TESTDATA"/labeled_container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr1_id="$output"

	ctrconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["metadata"]["name"] = "ctr2";obj["labels"]["group"] = "test";obj["labels"]["name"] = "ctr2";obj["labels"]["version"] = "v1.0.0"; json.dump(obj, sys.stdout)')
	echo "$ctrconfig" > "$TESTDATA"/labeled_container_redis.json
	run crictl create "$pod_id" "$TESTDATA"/labeled_container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr2_id="$output"

	ctrconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["metadata"]["name"] = "ctr3";obj["labels"]["group"] = "test";obj["labels"]["name"] = "ctr3";obj["labels"]["version"] = "v1.1.0"; json.dump(obj, sys.stdout)')
	echo "$ctrconfig" > "$TESTDATA"/labeled_container_redis.json
	run crictl create "$pod_id" "$TESTDATA"/labeled_container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr3_id="$output"

	run crictl ps --label "group=test" --label "name=ctr1" --label "version=v1.0.0" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "$ctr1_id" ]]
	run crictl ps --label "group=production" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
	run crictl ps --label "group=test" --label "version=v1.0.0" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr1_id" ]]
	[[ "$output" =~ "$ctr2_id" ]]
	[[ "$output" != "$ctr3_id" ]]
	run crictl ps --label "group=test" --quiet --all
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr1_id"  ]]
	[[ "$output" =~ "$ctr2_id"  ]]
	[[ "$output" =~ "$ctr3_id"  ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr metadata in list & status" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run crictl ps --id "$ctr_id" --output yaml --state created
	echo "$output"
	[ "$status" -eq 0 ]
	# TODO: expected value should not hard coded here
	[[ "$output" =~ "name: container1" ]]
	[[ "$output" =~ "attempt: 1" ]]

	run crictl inspect "$ctr_id" --output table
	echo "$output"
	[ "$status" -eq 0 ]
	# TODO: expected value should not hard coded here
	[[ "$output" =~ "Name: container1" ]]
	[[ "$output" =~ "Attempt: 1" ]]
}

@test "ctr execsync conflicting with conmon flags parsing" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" sh -c "echo hello world"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "hello world" ]]
}

@test "ctr execsync" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" echo HELLO
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "HELLO" ]]
	run crictl exec --sync --timeout 1 "$ctr_id" sleep 3
	echo "$output"
	[[ "$output" =~ "command timed out" ]]
	[ "$status" -ne 0 ]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr device add" {
	# In an user namespace we can only bind mount devices from the host, not mknod
	# https://github.com/opencontainers/runc/blob/master/libcontainer/rootfs_linux.go#L480-L481
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	cp "$TESTDATA"/container_redis_device.json "$newconfig"
	sed -i 's|"%containerdevicepath%"|"/dev/mynull"|' "$newconfig"

	sed -i 's|"%privilegedboolean%"|false|' "$newconfig"

	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" ls /dev/mynull
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "/dev/mynull" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "privileged ctr device add" {
	# In an user namespace we can only bind mount devices from the host, not mknod
	# https://github.com/opencontainers/runc/blob/master/libcontainer/rootfs_linux.go#L480-L481
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config_privileged.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	cp "$TESTDATA"/container_redis_device.json "$newconfig"
	sed -i 's|"%containerdevicepath%"|"/dev/mynull"|' "$newconfig"
	sed -i 's|"%privilegedboolean%"|true|' "$newconfig"

	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config_privileged.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" ls /dev/mynull
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "/dev/mynull" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "privileged ctr add duplicate device as host" {
	# In an user namespace we can only bind mount devices from the host, not mknod
	# https://github.com/opencontainers/runc/blob/master/libcontainer/rootfs_linux.go#L480-L481
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config_privileged.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	cp "$TESTDATA"/container_redis_device.json "$newconfig"
	sed -i 's|"%containerdevicepath%"|"/dev/random"|' "$newconfig"
	sed -i 's|"%privilegedboolean%"|true|' "$newconfig"

	run crictl create "$pod_id" "$newconfig" "$TESTDATA"/sandbox_config_privileged.json
	echo "$output"
	[ "$status" -ne 0 ]
}

@test "ctr hostname env" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl exec --sync "$ctr_id" env
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "HOSTNAME" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr execsync failure" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" doesnotexist
	echo "$output"
	[ "$status" -ne 0 ] || [ "$output" =~ "Exit code: 1" ]
	cleanup_ctrs
	cleanup_pods
	stop_crio
}

@test "ctr execsync exit code" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" false
	echo "$output"
	[ "$status" -ne 0 ]
}

@test "ctr execsync std{out,err}" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" echo hello0 stdout
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "hello0 stdout" ]]

	stderrconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["image"]["image"] = "quay.io/crio/stderr-test"; obj["command"] = ["/bin/sleep", "600"]; json.dump(obj, sys.stdout)')
	echo "$stderrconfig" > "$TESTDIR"/container_config_stderr.json
	run crictl create "$pod_id" "$TESTDIR"/container_config_stderr.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" stderr
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "this goes to stderr" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr stop idempotent" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stop "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl stop "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr caps drop" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	capsconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["security_context"]["capabilities"] = {u"add_capabilities": [], u"drop_capabilities": [u"mknod", u"kill", u"sys_chroot", u"setuid", u"setgid"]}; json.dump(obj, sys.stdout)')
	echo "$capsconfig" > "$TESTDIR"/container_config_caps.json
	run crictl create "$TESTDIR"/container_config_caps.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr with default list of capabilities from crio.conf" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync $ctr_id grep Cap /proc/1/status
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ 00000000002425fb ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr with list of capabilities given by user in crio.conf" {
	export CONTAINER_DEFAULT_CAPABILITIES="CHOWN,DAC_OVERRIDE,FSETID,FOWNER,NET_RAW,SETGID,SETUID"
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync $ctr_id grep Cap /proc/1/status
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ 00000000002020db ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "run ctr with image with Config.Volumes" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	start_crio
	run crictl pull gcr.io/k8s-testimages/redis:e2e
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	volumesconfig=$(cat "$TESTDATA"/container_redis.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["image"]["image"] = "gcr.io/k8s-testimages/redis:e2e"; obj["args"] = []; json.dump(obj, sys.stdout)')
	echo "$volumesconfig" > "$TESTDIR"/container_config_volumes.json
	run crictl create "$pod_id" "$TESTDIR"/container_config_volumes.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr oom" {
	if [[ "$CI" == "true" ]]; then
		skip "container tests don't support testing OOM"
	fi
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	oomconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["image"]["image"] = "quay.io/crio/oom"; obj["linux"]["resources"]["memory_limit_in_bytes"] = 25165824; obj["command"] = ["/oom"]; json.dump(obj, sys.stdout)')
	echo "$oomconfig" > "$TESTDIR"/container_config_oom.json
	run crictl create "$pod_id" "$TESTDIR"/container_config_oom.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	# Wait for container to OOM
	attempt=0
	while [ $attempt -le 100 ]; do
		attempt=$((attempt+1))
		run crictl inspect --output yaml "$ctr_id"
		echo "$output"
		[ "$status" -eq 0 ]
		if [[ "$output" =~ "OOMKilled" ]]; then
			break
		fi
		sleep 10
	done
	[[ "$output" =~ "OOMKilled" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr /etc/resolv.conf rw/ro mode" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_config_resolvconf.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
	run crictl create "$pod_id" "$TESTDATA"/container_config_resolvconf_ro.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run wait_until_exit "$ctr_id"
	[ "$status" -eq 0 ]
}

@test "ctr create with non-existent command" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	newconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["command"] = ["nonexistent"]; json.dump(obj, sys.stdout)')
	echo "$newconfig" > "$TESTDIR"/container_nonexistent.json
	run crictl create "$pod_id" "$TESTDIR"/container_nonexistent.json "$TESTDATA"/sandbox_config.json
	[ "$status" -ne 0 ]
	[[ "$output" =~ "executable file not found" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr create with non-existent command [tty]" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	newconfig=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["command"] = ["nonexistent"]; obj["tty"] = True; json.dump(obj, sys.stdout)')
	echo "$newconfig" > "$TESTDIR"/container_nonexistent.json
	run crictl create "$pod_id" "$TESTDIR"/container_nonexistent.json "$TESTDATA"/sandbox_config.json
	[ "$status" -ne 0 ]
	[[ "$output" =~ "executable file not found" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr update resources" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/memory/memory.limit_in_bytes"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "209715200" ]]
	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.shares"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "512" ]]
	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.cfs_period_us"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "10000" ]]
	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.cfs_quota_us"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "20000" ]]

	run crictl update --memory 524288000 --cpu-period 20000 --cpu-quota 10000 --cpu-share 256 "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/memory/memory.limit_in_bytes"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "524288000" ]]
	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.shares"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "256" ]]
	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.cfs_period_us"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "20000" ]]
	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpu/cpu.cfs_quota_us"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "10000" ]]
}

@test "ctr correctly setup working directory" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	notexistcwd=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["working_dir"] = "/thisshouldntexistatall"; json.dump(obj, sys.stdout)')
	echo "$notexistcwd" > "$TESTDIR"/container_cwd_notexist.json
	run crictl create "$pod_id" "$TESTDIR"/container_cwd_notexist.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	filecwd=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["working_dir"] = "/etc/passwd"; obj["metadata"]["name"] = "container2"; json.dump(obj, sys.stdout)')
	echo "$filecwd" > "$TESTDIR"/container_cwd_file.json
	run crictl create "$pod_id" "$TESTDIR"/container_cwd_file.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -ne 0 ]
	ctr_id="$output"
	[[ "$output" =~ "not a directory" ]]
}

@test "ctr execsync conflicting with conmon env" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis_env_custom.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec "$ctr_id" env
	echo "$output"
	echo "$status"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "acustompathinpath" ]]
	run crictl exec --sync "$ctr_id" env
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "acustompathinpath" ]]
}

@test "ctr resources" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpuset/cpuset.cpus"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "0" ]]
	run crictl exec --sync "$ctr_id" sh -c "cat /sys/fs/cgroup/cpuset/cpuset.mems"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "0" ]]
}

@test "ctr with non-root user has no effective capabilities" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	newconfig=$(cat "$TESTDATA"/container_redis.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["security_context"]["run_as_username"] = "redis"; json.dump(obj, sys.stdout)')
	echo "$newconfig" > "$TESTDIR"/container_user.json

	run crictl create "$pod_id" "$TESTDIR"/container_user.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" grep "CapEff:\s0000000000000000" /proc/1/status
	echo "$output"
	[ "$status" -eq 0 ]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr with low memory configured should not be created" {
	start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	low_mem_config=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["resources"]["memory_limit_in_bytes"] = 2000; json.dump(obj, sys.stdout)')
	echo "$low_mem_config" > "$TESTDIR"/container_config_low_mem.json
	run crictl create "$pod_id" "$TESTDIR"/container_config_low_mem.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ ! "$status" -eq 0 ]
	ctr_id="$output"
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr expose metrics with default port" {
	# start crio with default port 9090
	port="9090"
	start_crio_metrics
	# ensure metrics port is listening
	listened=$(check_metrics_port $port)
	if [[ "$listened" -ne 0 ]]; then
		skip "$CONTAINER_METRICS_PORT is not listening"
	fi

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	[ "$status" -eq 0 ]

	# get metrics
	run curl http://localhost:$port/metrics -k
	[ "$status" -eq 0 ]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "ctr expose metrics with custom port" {
	# start crio with custom port
	port="4321"
	CONTAINER_METRICS_PORT=$port start_crio_metrics
	# ensure metrics port is listening
	listened=$(check_metrics_port $port)
	if [[ "$listened" -ne 0 ]]; then
		skip "$CONTAINER_METRICS_PORT is not listening"
	fi

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	[ "$status" -eq 0 ]

	# get metrics
	run curl http://localhost:$port/metrics -k
	[ "$status" -eq 0 ]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}


@test "privileged ctr -- check for rw mounts" {
	start_crio

	run crictl runp "$TESTDATA"/sandbox_config_privileged.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	[ "$status" -eq 0 ]

	run crictl exec "$ctr_id" grep ro\, /proc/mounts
	[ "$status" -eq 0 ]
	[[ "$output" =~ "tmpfs /sys/fs/cgroup tmpfs" ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}
