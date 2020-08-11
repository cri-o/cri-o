#!/usr/bin/env bats

load helpers

function setup() {
	if ! "$CHECKSECCOMP_BINARY"; then
		skip "seccomp is not enabled"
	fi

	setup_test
}

function teardown() {
	cleanup_test
}

# 1. test running with ctr unconfined
# test that we can run with a syscall which would be otherwise blocked
@test "ctr seccomp profiles unconfined" {
	sed -e 's/"chmod",//' "$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	export CONTAINER_SECCOMP_PROFILE=""$TESTDIR"/seccomp_profile1.json"
	start_crio

	sed -e 's/%VALUE%/unconfined/g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDIR"/seccomp1.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" chmod 777 .
	echo "$output"
	[ "$status" -eq 0 ]
}

# 2. test running with ctr runtime/default
# test that we cannot run with a syscall blocked by the default seccomp profile
@test "ctr seccomp profiles runtime/default" {
	sed -e 's/"chmod",//' "$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	export CONTAINER_SECCOMP_PROFILE=""$TESTDIR"/seccomp_profile1.json"
	start_crio

	sed -e 's/%VALUE%/runtime\/default/g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp2.json
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDIR"/seccomp2.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" chmod 777 .
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" =~ "Operation not permitted" ]]
}

# 3. test running with ctr unconfined and profile empty
# test that we can run with a syscall which would be otherwise blocked
@test "ctr seccomp profiles unconfined by empty field" {
	sed -e 's/"chmod",//' "$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	export CONTAINER_SECCOMP_PROFILE=""$TESTDIR"/seccomp_profile1.json"
	start_crio

	sed -e 's/%VALUE%//g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDIR"/seccomp1.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" chmod 777 .
	echo "$output"
	[ "$status" -eq 0 ]
}

# 4. test running with ctr wrong profile name
@test "ctr seccomp profiles wrong profile name" {
	sed -e 's/"chmod",//' "$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	export CONTAINER_SECCOMP_PROFILE=""$TESTDIR"/seccomp_profile1.json"
	start_crio

	sed -e 's/%VALUE%/wontwork/g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDIR"/seccomp1.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[[ "$status" -ne 0 ]]
	[[ "$output" =~ "unknown seccomp profile option:"  ]]
	[[ "$output" =~ "wontwork"  ]]
}

# 5. test running with ctr localhost/profile_name
@test "ctr seccomp profiles localhost/profile_name" {
	start_crio

	sed -e 's/"chmod",//' "$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	sed -e 's@%VALUE%@localhost/'"$TESTDIR"'/seccomp_profile1.json@g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp1.json
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDIR"/seccomp1.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" chmod 777 .
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" =~ "Operation not permitted" ]]
}

# 6. test running with ctr docker/default
# test that we cannot run with a syscall blocked by the default seccomp profile
@test "ctr seccomp profiles docker/default" {
	sed -e 's/"chmod",//' "$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmod",//' "$TESTDIR"/seccomp_profile1.json
	sed -i 's/"fchmodat",//g' "$TESTDIR"/seccomp_profile1.json

	export CONTAINER_SECCOMP_PROFILE=""$TESTDIR"/seccomp_profile1.json"
	start_crio

	sed -e 's/%VALUE%/docker\/default/g' "$TESTDATA"/container_config_seccomp.json > "$TESTDIR"/seccomp2.json
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDIR"/seccomp2.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" chmod 777 .
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" =~ "Operation not permitted" ]]
}
