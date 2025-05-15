#!/usr/bin/env bats

load helpers

function setup() {
	if ! "$CHECKSECCOMP_BINARY"; then
		skip "seccomp is not enabled"
	fi

	setup_test

	sed -e 's/"chmod",//' -e 's/"fchmod",//' -e 's/"fchmodat",//g' \
		"$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/seccomp_profile1.json

	CONTAINER_SECCOMP_PROFILE="$TESTDIR"/seccomp_profile1.json start_crio
}

function teardown() {
	cleanup_test
}

# SecurityProfile_RuntimeDefault = 0
# SecurityProfile_Unconfined = 1
# SecurityProfile_Localhost = 2

# test that we can run with a syscall which would be otherwise blocked
@test "ctr seccomp profiles unconfined" {
	jq '	  .linux.security_context.seccomp.profile_type = 1' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/seccomp.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/seccomp.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl exec --sync "$ctr_id" chmod 777 .
}

# test that we cannot run with a syscall blocked by the default seccomp profile
@test "ctr seccomp profiles runtime/default" {
	jq '	  .linux.security_context.seccomp.profile_type = 0' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/seccomp.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/seccomp.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	run ! crictl exec --sync "$ctr_id" chmod 777 .
}

@test "ctr seccomp profiles wrong profile name" {
	jq '	  .linux.security_context.seccomp.profile_type = 2 | .linux.security_context.seccomp.localhost_ref = "wontwork"' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/seccomp.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	run ! crictl create "$pod_id" "$TESTDIR"/seccomp.json "$TESTDATA"/sandbox_config.json
	[[ "$output" =~ "no such file or directory" ]]
	[[ "$output" =~ "wontwork" ]]
}

@test "ctr seccomp profiles localhost profile name" {
	jq '	  .linux.security_context.seccomp.profile_type = 2 | .linux.security_context.seccomp.localhost_ref = "'"$TESTDIR"'/seccomp_profile1.json"' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/seccomp.json
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/seccomp.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	run ! crictl exec --sync "$ctr_id" chmod 777 .
}

# test that we can run with a syscall which would be otherwise blocked
@test "ctr seccomp overrides unconfined profile with runtime/default when overridden" {
	export CONTAINER_SECCOMP_PROFILE="$TESTDIR"/seccomp_profile1.json
	restart_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl exec --sync "$ctr_id" chmod 777 .
}

@test "ctr seccomp profiles runtime/default blocks unshare" {
	unset CONTAINER_SECCOMP_PROFILE
	restart_crio

	jq '.linux.security_context.seccomp.profile_type = 0' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container.json"

	ctr_id=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")
	run ! crictl exec --sync "$ctr_id" /bin/sh -c "unshare"
}

@test "ctr seccomp profiles runtime/default blocks clone creating namespaces" {
	unset CONTAINER_SECCOMP_PROFILE
	restart_crio

	jq --arg TESTDATA "$TESTDATA" '.linux.security_context.seccomp.profile_type = 0 |
          .mounts = [{
            host_path: $TESTDATA,
            container_path: "/testdata",
          }]' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container.json"

	ctr_id=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")
	crictl exec --sync "$ctr_id" /usr/bin/cp /testdata/clone-ns.c /
	crictl exec --sync "$ctr_id" /usr/bin/gcc /clone-ns.c -o /usr/bin/clone-ns
	run crictl exec --sync "$ctr_id" /usr/bin/clone-ns with_flags
	[[ "$output" =~ Operation\ not\ permitted.* ]]
}

@test "ctr seccomp profiles runtime/default allows clone not creating namespaces" {
	unset CONTAINER_SECCOMP_PROFILE
	restart_crio

	jq --arg TESTDATA "$TESTDATA" '.linux.security_context.seccomp.profile_type = 0 |
          .mounts = [{
            host_path: $TESTDATA,
            container_path: "/testdata",
          }]' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container.json"

	ctr_id=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")
	crictl exec --sync "$ctr_id" /usr/bin/cp /testdata/clone-ns.c /
	crictl exec --sync "$ctr_id" /usr/bin/gcc /clone-ns.c -o /usr/bin/clone-ns
	crictl exec --sync "$ctr_id" /usr/bin/clone-ns without_flags
}

@test "ctr seccomp profiles runtime/default with SYS_ADMIN capability allows clone creating namespaces" {
	unset CONTAINER_SECCOMP_PROFILE
	export CONTAINER_ADD_INHERITABLE_CAPABILITIES=true
	restart_crio

	jq --arg TESTDATA "$TESTDATA" '.linux.security_context.seccomp.profile_type = 0 |
	        .linux.security_context.capabilities.add_capabilities = ["SYS_ADMIN"] |
          .mounts = [{
            host_path: $TESTDATA,
            container_path: "/testdata",
          }]' \
		"$TESTDATA/container_sleep.json" > "$TESTDIR/container.json"

	ctr_id=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")
	crictl exec --sync "$ctr_id" /usr/bin/cp /testdata/clone-ns.c /
	crictl exec --sync "$ctr_id" /usr/bin/gcc /clone-ns.c -o /usr/bin/clone-ns
	crictl exec --sync "$ctr_id" /usr/bin/clone-ns with_flags
}

@test "ctr seccomp profile for privileged sandbox and container" {
	# shellcheck disable=SC2030,SC2031
	export CONTAINER_PRIVILEGED_SECCOMP_PROFILE="$TESTDIR/seccomp_profile1.json"
	restart_crio

	POD_JSON="$TESTDIR/sandbox.json"
	CTR_JSON="$TESTDIR/container.json"

	jq '.linux.security_context.privileged = true' \
		"$TESTDATA/sandbox_config.json" > "$POD_JSON"

	jq '.linux.security_context.privileged = true' \
		"$TESTDATA/container_sleep.json" > "$CTR_JSON"

	POD_ID=$(crictl runp "$POD_JSON")
	CTR_ID=$(crictl create "$POD_ID" "$CTR_JSON" "$POD_JSON")
	crictl start "$CTR_ID"

	crictl inspectp "$POD_ID" | jq -e '.info.runtimeSpec.linux.seccomp != null'
	crictl inspect "$CTR_ID" | jq -e '.info.runtimeSpec.linux.seccomp != null'
	run ! crictl exec --sync "$CTR_ID" chmod 777 .
}

@test "ctr seccomp profile for privileged sandbox only " {
	# shellcheck disable=SC2030,SC2031
	export CONTAINER_PRIVILEGED_SECCOMP_PROFILE="$TESTDIR/seccomp_profile1.json"
	restart_crio

	POD_JSON="$TESTDIR/sandbox.json"
	CTR_JSON="$TESTDATA/container_sleep.json"

	jq '.linux.security_context.privileged = true' \
		"$TESTDATA/sandbox_config.json" > "$POD_JSON"

	POD_ID=$(crictl runp "$POD_JSON")
	CTR_ID=$(crictl create "$POD_ID" "$CTR_JSON" "$POD_JSON")
	crictl start "$CTR_ID"

	crictl inspectp "$POD_ID" | jq -e '.info.runtimeSpec.linux.seccomp != null'
	crictl inspect "$CTR_ID" | jq -e '.info.runtimeSpec.linux.seccomp == null'
	crictl exec --sync "$CTR_ID" chmod 777 .
}

@test "ctr seccomp profile for privileged container but not existing" {
	# shellcheck disable=SC2030,SC2031
	export CONTAINER_PRIVILEGED_SECCOMP_PROFILE="not-existing"
	restart_crio

	jq '.linux.security_context.privileged = true' \
		"$TESTDATA/sandbox_config.json" > "$TESTDIR/sandbox.json"

	run ! crictl runp "$TESTDIR/sandbox.json"
}

@test "ctr privileged seccomp profile not existing and not required" {
	# shellcheck disable=SC2030,SC2031
	export CONTAINER_PRIVILEGED_SECCOMP_PROFILE="not-existing"
	restart_crio

	POD_JSON="$TESTDATA/sandbox_config.json"
	CTR_JSON="$TESTDATA/container_sleep.json"

	POD_ID=$(crictl runp "$POD_JSON")
	CTR_ID=$(crictl create "$POD_ID" "$CTR_JSON" "$POD_JSON")
	crictl start "$CTR_ID"

	crictl inspectp "$POD_ID" | jq -e '.info.runtimeSpec.linux.seccomp == null'
	crictl inspect "$CTR_ID" | jq -e '.info.runtimeSpec.linux.seccomp == null'
	crictl exec --sync "$CTR_ID" chmod 777 .
}
