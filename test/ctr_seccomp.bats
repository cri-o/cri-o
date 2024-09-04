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
