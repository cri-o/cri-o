#!/usr/bin/env bats

load helpers

# These values come from https://github.com/containers/storage/blob/be5932a4d81cc01a1cf9cab0fb4cbf9c9892ef5c/store.go#L3463..L3471
# Since the test suite doesn't specify different values
AUTO_USERNS_USER="containers"
AUTO_USERNS_MAX_SIZE="65536"
FIRST_UID=$(grep $AUTO_USERNS_USER /etc/subuid | cut -d : -f 2)
FIRST_GID=$(grep $AUTO_USERNS_USER /etc/subgid | cut -d : -f 2)

function setup() {
	setup_test
	sboxconfig="$TESTDIR/sandbox_config.json"
	ctrconfig="$TESTDIR/container_config.json"
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.userns-mode"
	start_crio
}

function teardown() {
	cleanup_test
}

@test "userns annotation auto should succeed" {
	jq '      .annotations."io.kubernetes.cri-o.userns-mode" = "auto"' \
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
	jq '      .annotations."io.kubernetes.cri-o.userns-mode" = "auto:keep-id=true;map-to-root=true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	run ! crictl runp "$sboxconfig"
}

@test "userns annotation auto should map host run_as_user" {
	jq '      .annotations."io.kubernetes.cri-o.userns-mode" = "auto"' \
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
