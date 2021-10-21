#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "selinux label level=s0 is sufficient" {
	start_crio

	jq '	  .linux.security_context.selinux_options = {"level": "s0"}' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox.json)
	crictl start "$ctr_id"
}

@test "selinux skips relabeling for super priviliged container" {
	if [[ $(getenforce) != "Enforcing" ]]; then
		skip "not enforcing"
	fi
	VOLUME="$TESTDIR"/dir
	mkdir -p "$VOLUME"

	# shellcheck disable=SC2012
	OLDLABEL=$(ls -dZ "$VOLUME" | awk '{ printf $1 }')

	start_crio

	jq '.linux.security_context.selinux_options = {"type": "spc_t"}' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	jq --arg path "$VOLUME" \
		'.mounts = [{
			host_path: $path,
			container_path: "/tmp/path",
			selinux_relabel: true
		}]' \
		"$TESTDATA"/container_redis.json > "$TESTDIR"/container.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container.json "$TESTDIR"/sandbox.json)

	crictl rm "$ctr_id"

	# shellcheck disable=SC2012
	NEWLABEL=$(ls -dZ "$VOLUME" | awk '{ printf $1 }')

	[[ "$OLDLABEL" == "$NEWLABEL" ]]
}
