#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

@test "ctr remove" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run ocic ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr remove --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod stop --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "ctr lifecycle" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic pod list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr stop --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr remove --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod stop --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

# regression test for #127
@test "ctrs status for a pod" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]

	run ocic ctr list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "${output}" != "" ]]

	printf '%s\n' "$output" | while IFS= read -r id
	do
		run ocic ctr status --id "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done

	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "ctr list filtering" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json --name pod1
	echo "$output"
	[ "$status" -eq 0 ]
	pod1_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr1_id="$output"
	run ocic ctr start --id "$ctr1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod create --config "$TESTDATA"/sandbox_config.json --name pod2
	echo "$output"
	[ "$status" -eq 0 ]
	pod2_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod2_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr2_id="$output"
	run ocic pod create --config "$TESTDATA"/sandbox_config.json --name pod3
	echo "$output"
	[ "$status" -eq 0 ]
	pod3_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod3_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr3_id="$output"
	run ocic ctr start --id "$ctr3_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr stop --id "$ctr3_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list --id "$ctr1_id" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr1_id"  ]]
	run ocic ctr list --id "$ctr2_id" --pod "$pod2_id" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr2_id"  ]]
	run ocic ctr list --id "$ctr2_id" --pod "$pod3_id" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
	run ocic ctr list --state created --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr2_id"  ]]
	run ocic ctr list --state running --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr1_id"  ]]
	run ocic ctr list --state stopped --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr3_id"  ]]
	run ocic ctr list --pod "$pod1_id" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr1_id"  ]]
	run ocic ctr list --pod "$pod2_id" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr2_id"  ]]
	run ocic ctr list --pod "$pod3_id" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr3_id"  ]]
	run ocic pod remove --id "$pod1_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod2_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod3_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "ctr list label filtering" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id" --name ctr1 --label "a=b" --label "c=d" --label "e=f"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr1_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id" --name ctr2 --label "a=b" --label "c=d"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr2_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id" --name ctr3 --label "a=b"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr3_id="$output"
	run ocic ctr list --label "tier=backend" --label "a=b" --label "c=d" --label "e=f" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr1_id"  ]]
	run ocic ctr list --label "tier=frontend" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == "" ]]
	run ocic ctr list --label "a=b" --label "c=d" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr1_id"  ]]
	[[ "$output" =~ "$ctr2_id"  ]]
	run ocic ctr list --label "a=b" --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" != "" ]]
	[[ "$output" =~ "$ctr1_id"  ]]
	[[ "$output" =~ "$ctr2_id"  ]]
	[[ "$output" =~ "$ctr3_id"  ]]
	run ocic pod remove --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "ctr metadata in list & status" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_config.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"

	run ocic ctr list --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	# TODO: expected value should not hard coded here
	[[ "$output" =~ "Name: container1" ]]
	[[ "$output" =~ "Attempt: 1" ]]

	run ocic ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	# TODO: expected value should not hard coded here
	[[ "$output" =~ "Name: container1" ]]
	[[ "$output" =~ "Attempt: 1" ]]

	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "ctr execsync" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run ocic ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr execsync --id "$ctr_id" echo HELLO
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "HELLO" ]]
	run ocic ctr execsync --id "$ctr_id" --timeout 1 sleep 10
	echo "$output"
	[[ "$output" =~ "command timed out" ]]
	run ocic pod remove --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "ctr execsync failure" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run ocic ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr execsync --id "$ctr_id" doesnotexist
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" =~ "executable file not found in" ]]

	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "ctr stop idempotent" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run ocic ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr stop --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr stop --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]

	cleanup_ctrs
	cleanup_pods
	stop_ocid
}
