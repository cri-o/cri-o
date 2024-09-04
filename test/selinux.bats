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
	if ! is_selinux_enforcing; then
		skip "not enforcing"
	fi

	# RHEL/CentOS 7's container-selinux package replaces container_file_t with svirt_sandbox_file_t
	# under the hood. This causes the annotation to not work correctly.
	if is_rhel_7; then
		skip "fails on RHEL 7 or earlier"
	fi

	VOLUME="$TESTDIR"/dir
	FILE="$VOLUME"/file
	mkdir "$VOLUME"
	touch "$FILE"

	setup_crio
	create_runtime_with_allowed_annotation "selinux" "io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel"
	start_crio_no_setup

	jq '	  .linux.security_context.selinux_options = {"level": "s0:c200,c100"}
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

	# shellcheck disable=SC2010
	oldlabel=$(ls -Z "$FILE" | grep -o '[a-z,_]*_u:[a-z,_]*_r:[a-z,_]*_t:[c,s,0-9,:,\,]* ')

	# Label file, but not top dir. This will show us the directory was not relabeled (as expected)
	chcon --reference "$TESTDIR"/container.json "$FILE" # || \

	# shellcheck disable=SC2010
	label=$(ls -Z "$FILE" | grep -o '[a-z,_]*_u:[a-z,_]*_r:[a-z,_]*_t:[c,s,0-9,:,\,]* ')
	[[ "$oldlabel" != "$label" ]]

	# Recreate. Since top level is already labeled right, there won't be a relabel.
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container.json "$TESTDIR"/sandbox.json)
	# shellcheck disable=SC2010
	newlabel=$(ls -Z "$FILE" | grep -o '[a-z,_]*_u:[a-z,_]*_r:[a-z,_]*_t:[c,s,0-9,:,\,]* ')
	[[ "$label" == "$newlabel" ]]

	crictl rm "$ctr_id"

	# Recreate with same context but categories in different order.  Also should not relabel.
	toplabel="system_u:object_r:container_file_t:s0:c100,c200"
	chcon "$toplabel" "$VOLUME"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container.json "$TESTDIR"/sandbox.json)
	# shellcheck disable=SC2010
	newlabel=$(ls -Z "$FILE" | grep -o '[a-z,_]*_u:[a-z,_]*_r:[a-z,_]*_t:[c,s,0-9,:,\,]* ')
	[[ "$label" == "$newlabel" ]]

}

@test "selinux skips relabeling for super privileged container" {
	if ! is_selinux_enforcing; then
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
