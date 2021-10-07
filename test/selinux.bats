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

@test "selinux skips relabeling if TrySkipVolumeSELinuxLabel annotation is present" {
	if [[ $(getenforce) != "Enforcing" ]]; then
		skip "not enforcing"
	fi
	VOLUME="$TESTDIR"/dir
	mkdir "$VOLUME"
	touch "$VOLUME"/file

	create_runtime_with_allowed_annotation "selinux" "io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel"
	start_crio

	jq '	  .linux.security_context.selinux_options = {"level": "s0:c100,c200"}
		|  .annotations["io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel"] = "true"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	jq --arg path "$VOLUME" \
		'  .mounts = [ {
			host_path: $path,
			container_path: "/tmp/path",
			selinux_relabel: true
		} ]' \
		"$TESTDATA"/container_redis.json > "$TESTDIR"/container.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container.json "$TESTDIR"/sandbox.json)

	crictl rm "$ctr_id"

	# shellcheck disable=SC2012
	oldlabel=$(ls -Z "$VOLUME" | awk '{ printf $1 }')

	# Label file, but not top dir. This will show us the directory was not relabeled (as expected)
	chcon --reference "$TESTDIR"/container.json "$VOLUME"/file # || \

	# shellcheck disable=SC2012
	label=$(ls -Z "$VOLUME" | awk '{ printf $1 }')
	[[ "$oldlabel" != "$label" ]]

	# Recreate. Since top level is already labeled right, there won't be a relabel.
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container.json "$TESTDIR"/sandbox.json)
	# shellcheck disable=SC2012
	newlabel=$(ls -Z "$VOLUME" | awk '{ printf $1 }')
	[[ "$label" == "$newlabel" ]]
}
