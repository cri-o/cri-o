#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

# 1. test running with ctr unconfined
# test that we can run with a syscall which would be otherwise blocked
@test "ctr seccomp profiles unconfined" {
	# this test requires seccomp, so skip this test if seccomp is not enabled.
	enabled=$(is_seccomp_enabled)
	if [[ "$enabled" -eq 0 ]]; then
		skip "skip this test since seccomp is not enabled."
	fi

	sed -e 's/"chmod",//' "$SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	start_crio "$TESTDIR"/seccomp_profile1.json

	sed -e 's/%VALUE%/unconfined/g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	run crioctl pod run --name seccomp1 --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crioctl ctr create --name testname --config "$TESTDIR"/seccomp1.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crioctl ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crioctl ctr execsync --id "$ctr_id" chmod 777 .
	echo "$output"
	[ "$status" -eq 0 ]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}

# 2. test running with ctr runtime/default
# test that we cannot run with a syscall blocked by the default seccomp profile
@test "ctr seccomp profiles runtime/default" {
	# this test requires seccomp, so skip this test if seccomp is not enabled.
	enabled=$(is_seccomp_enabled)
	if [[ "$enabled" -eq 0 ]]; then
		skip "skip this test since seccomp is not enabled."
	fi

	sed -e 's/"chmod",//' "$SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	start_crio "$TESTDIR"/seccomp_profile1.json

	sed -e 's/%VALUE%/runtime\/default/g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp2.json
	run crioctl pod run --name seccomp2 --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crioctl ctr create --name testname2 --config "$TESTDIR"/seccomp2.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crioctl ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crioctl ctr execsync --id "$ctr_id" chmod 777 .
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "Exit code: 1" ]]
	[[ "$output" =~ "Operation not permitted" ]]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}

# 3. test running with ctr unconfined and profile empty
# test that we can run with a syscall which would be otherwise blocked
@test "ctr seccomp profiles unconfined by empty field" {
	# this test requires seccomp, so skip this test if seccomp is not enabled.
	enabled=$(is_seccomp_enabled)
	if [[ "$enabled" -eq 0 ]]; then
		skip "skip this test since seccomp is not enabled."
	fi

	sed -e 's/"chmod",//' "$SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	start_crio "$TESTDIR"/seccomp_profile1.json

	sed -e 's/%VALUE%//g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	run crioctl pod run --name seccomp1 --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crioctl ctr create --name testname --config "$TESTDIR"/seccomp1.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crioctl ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crioctl ctr execsync --id "$ctr_id" chmod 777 .
	echo "$output"
	[ "$status" -eq 0 ]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}

# 4. test running with ctr wrong profile name
@test "ctr seccomp profiles wrong profile name" {
	# this test requires seccomp, so skip this test if seccomp is not enabled.
	enabled=$(is_seccomp_enabled)
	if [[ "$enabled" -eq 0 ]]; then
		skip "skip this test since seccomp is not enabled."
	fi

	sed -e 's/"chmod",//' "$SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	start_crio "$TESTDIR"/seccomp_profile1.json

	sed -e 's/%VALUE%/wontwork/g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	run crioctl pod run --name seccomp1 --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crioctl ctr create --name testname --config "$TESTDIR"/seccomp1.json --pod "$pod_id"
	echo "$output"
	[[ "$status" -ne 0 ]]
	[[ "$output" =~ "unknown seccomp profile option:"  ]]
	[[ "$output" =~ "wontwork"  ]]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}

# 5. test running with ctr localhost/profile_name
@test "ctr seccomp profiles localhost/profile_name" {
	# this test requires seccomp, so skip this test if seccomp is not enabled.
	enabled=$(is_seccomp_enabled)
	if [[ "$enabled" -eq 0 ]]; then
		skip "skip this test since seccomp is not enabled."
	fi

	start_crio

	sed -e 's/"chmod",//' "$SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	sed -e 's@%VALUE%@localhost/'"$TESTDIR"'/seccomp_profile1.json@g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	run crioctl pod run --name seccomp1 --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crioctl ctr create --name testname --config "$TESTDIR"/seccomp1.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crioctl ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crioctl ctr execsync --id "$ctr_id" chmod 777 .
	[ "$status" -eq 0 ]
	[[ "$output" =~ "Exit code: 1" ]]
	[[ "$output" =~ "Operation not permitted" ]]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}

# 6. test running with ctr docker/default
# test that we cannot run with a syscall blocked by the default seccomp profile
@test "ctr seccomp profiles runtime/default" {
	# this test requires seccomp, so skip this test if seccomp is not enabled.
	enabled=$(is_seccomp_enabled)
	if [[ "$enabled" -eq 0 ]]; then
		skip "skip this test since seccomp is not enabled."
	fi

	sed -e 's/"chmod",//' "$SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	start_crio "$TESTDIR"/seccomp_profile1.json

	sed -e 's/%VALUE%/docker\/default/g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp2.json
	run crioctl pod run --name seccomp2 --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crioctl ctr create --name testname2 --config "$TESTDIR"/seccomp2.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crioctl ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crioctl ctr execsync --id "$ctr_id" chmod 777 .
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "Exit code: 1" ]]
	[[ "$output" =~ "Operation not permitted" ]]

	cleanup_ctrs
	cleanup_pods
	stop_crio
}
