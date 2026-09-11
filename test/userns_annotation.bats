#!/usr/bin/env bats

load helpers

# These values come from https://github.com/containers/container-libs/blob/71ca11ca55f00cbf9c033f29f6d0cbafb76415e4/storage/store.go#L3815-L3823
# Since the test suite doesn't specify different values
AUTO_USERNS_USER="containers"
AUTO_USERNS_MAX_SIZE="65536"
FIRST_UID=$(grep $AUTO_USERNS_USER /etc/subuid | cut -d : -f 2)
FIRST_GID=$(grep $AUTO_USERNS_USER /etc/subgid | cut -d : -f 2)

function setup() {
	setup_test
	sboxconfig="$TESTDIR/sandbox_config.json"
	ctrconfig="$TESTDIR/container_config.json"
	create_workload_with_allowed_annotation "userns-mode.crio.io"
	start_crio
}

function teardown() {
	cleanup_test
}

@test "userns annotation auto should succeed" {
	jq '      .annotations."userns-mode.crio.io" = "auto"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$sboxconfig")

	pid=$(crictl inspect "$ctr_id" | jq .info.pid)
	cat /proc/"$pid"/uid_map
	# running auto will allocate the first available uid in the range allocated
	# to the user AUTO_USERNS_USER
	tr -s " " < /proc/"$pid"/uid_map | grep -oq "0 $FIRST_UID $AUTO_USERNS_MAX_SIZE"
	tr -s " " < /proc/"$pid"/gid_map | grep -oq "0 $FIRST_GID $AUTO_USERNS_MAX_SIZE"
}

@test "userns annotation auto with keep-id and map-to-root should fail" {
	jq '      .annotations."userns-mode.crio.io" = "auto:keep-id=true;map-to-root=true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	run ! crictl runp "$sboxconfig"
}

@test "userns annotation auto should map host run_as_user" {
	jq '      .annotations."userns-mode.crio.io" = "auto"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq '	.linux.security_context.run_as_user.value = 1234
       |	.linux.security_context.run_as_group.value = 1234' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	pid=$(crictl inspect "$ctr_id" | jq .info.pid)
	# the user outside the userns should be 101234
	stat -c "%u" /proc/"$pid"/ | grep -qo $((FIRST_UID + 1234))
	stat -c "%g" /proc/"$pid"/ | grep -qo $((FIRST_GID + 1234))

	# the user inside the userns should be 1234
	[[ $(crictl exec "$ctr_id" id -u) == "1234" ]]
	[[ $(crictl exec "$ctr_id" id -g) == "1234" ]]
}

# When the kubelet UserNamespacesSupport feature gate is enabled, pods without
# hostUsers: false are sent userns_options mode NODE (2). The CRI-O userns-mode
# annotation should still take effect in that case.
@test "userns annotation should take precedence over userns_options mode=NODE" {
	jq '      .annotations."userns-mode.crio.io" = "private:uidmapping=0:100000:65536;gidmapping=0:200000:65536"
	 |	.linux.security_context.run_as_user.value = 0
	 |	.linux.security_context.namespace_options.userns_options = {"mode": 2}' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq '      .linux.security_context.namespace_options.userns_options = {"mode": 2}' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	pid=$(crictl inspect "$ctr_id" | jq .info.pid)
	tr -s " " < /proc/"$pid"/uid_map | grep -q "0 100000 65536"
	tr -s " " < /proc/"$pid"/gid_map | grep -q "0 200000 65536"
}

@test "uid mappings config should take precedence over userns_options mode=NODE" {
	CONTAINER_UID_MAPPINGS="0:100000:65536" CONTAINER_GID_MAPPINGS="0:200000:65536" restart_crio

	jq '      .linux.security_context.namespace_options.userns_options = {"mode": 2}' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq '      .linux.security_context.namespace_options.userns_options = {"mode": 2}' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	pid=$(crictl inspect "$ctr_id" | jq .info.pid)
	tr -s " " < /proc/"$pid"/uid_map | grep -q "0 100000 65536"
	tr -s " " < /proc/"$pid"/gid_map | grep -q "0 200000 65536"
}

@test "userns_options mode NODE without annotation should use the host user namespace" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userns enabled"
	fi

	jq '      .linux.security_context.namespace_options.userns_options = {"mode": 2}' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	jq '      .linux.security_context.namespace_options.userns_options = {"mode": 2}' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl run "$ctrconfig" "$sboxconfig")

	pid=$(crictl inspect "$ctr_id" | jq .info.pid)
	[[ "$(readlink /proc/"$pid"/ns/user)" == "$(readlink /proc/self/ns/user)" ]]
}
