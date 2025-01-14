#!/usr/bin/env bats

# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "ensure correct hostname" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec --sync "$ctr_id" sh -c "hostname")
	[[ "$output" == *"crictl_host"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "echo \$HOSTNAME")
	[[ "$output" == *"crictl_host"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "cat /etc/hostname")
	[[ "$output" == *"crictl_host"* ]]
}

@test "ensure correct hostname for hostnetwork:true" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	start_crio

	jq '	  .linux.security_context.namespace_options.network = 2
		| del(.annotations)
		| del(.hostname)' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_hostnetwork.json

	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox_hostnetwork.json)

	output=$(crictl exec --sync "$ctr_id" sh -c "hostname")
	[[ "$output" == *"$HOSTNAME"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "echo \$HOSTNAME")
	[[ "$output" == *"$HOSTNAME"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "cat /etc/hostname")
	[[ "$output" == *"$HOSTNAME"* ]]
}

@test "Check for valid pod netns CIDR" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	output=$(crictl exec --sync "$ctr_id" ip addr show dev eth0 scope global)
	[[ "$output" = *" inet $POD_IPV4_CIDR_START"* ]]
	[[ "$output" = *" inet6 $POD_IPV6_CIDR_START"* ]]
}

@test "Ensure correct CNI plugin namespace/name/container-id arguments" {
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}" CNI_TYPE="cni_plugin_helper.bash" start_crio

	crictl runp "$TESTDATA"/sandbox_config.json

	# shellcheck disable=SC1090,SC1091
	. "$TESTDIR"/plugin_test_args.out

	[ "$FOUND_CNI_CONTAINERID" != "redhat.test.crio" ]
	[ "$FOUND_CNI_CONTAINERID" != "podsandbox1" ]
	[ "$FOUND_K8S_POD_NAMESPACE" = "redhat.test.crio" ]
	[ "$FOUND_K8S_POD_NAME" = "podsandbox1" ]
	[ "$FOUND_K8S_POD_UID" = "redhat-test-crio" ]
}

@test "Connect to pod hostport from the host" {
	if is_cgroup_v2; then
		skip "node configured with cgroupv2 flakes this test sometimes"
	fi
	start_crio

	pod_config="$TESTDIR"/sandbox_config.json
	jq '	  .port_mappings = [ {
			protocol: 0,
			container_port: 80,
			host_port: 4888
		} ]
		| .hostname = "very.unique.name" ' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"

	ctr_config="$TESTDIR"/container_config.json
	jq '	  .command = [ "/bin/nc", "-ll", "-p", "80", "-e", "/bin/hostname" ]' \
		"$TESTDATA"/container_config.json > "$ctr_config"

	crictl run "$ctr_config" "$pod_config"

	host_ip=$(get_host_ip)
	output=$(nc -w 5 "$host_ip" 4888 < /dev/null)
	[ "$output" = "very.unique.name" ]
}

# ensure that the server cleaned up sandbox networking
# if the sandbox failed after network setup
function check_networking() {
	# shellcheck disable=SC2010
	if ls /var/lib/cni/networks/"$CNI_DEFAULT_NETWORK" | grep -Ev '^lock|^last_reserved_ip'; then
		echo "unexpected networks found" 1>&2
		exit 1
	fi
}

@test "Clean up network if pod sandbox fails" {
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

	# TODO FIXME find a way for sandbox setup to fail if manage ns is true
	CONMON_BINARY="$TESTDIR"/conmon
	cp "$(command -v conmon)" "$CONMON_BINARY"
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}"
	CONTAINER_DROP_INFRA_CTR=false start_crio

	# make conmon non-executable to cause the sandbox setup to fail after
	# networking has been configured
	chmod 0644 "$CONMON_BINARY"
	run ! crictl runp "$TESTDATA"/sandbox_config.json

	check_networking
}

@test "Clean up network if pod sandbox fails after plugin success" {
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}"
	CNI_TYPE="cni_plugin_helper.bash" setup_crio
	echo "DEBUG_ARGS=malformed-result" > "$TESTDIR"/cni_plugin_helper_input.env
	start_crio_no_setup
	check_images

	run ! crictl runp "$TESTDATA"/sandbox_config.json

	check_networking
}

@test "Clean up network if pod sandbox gets killed" {
	CONTAINER_DROP_INFRA_CTR=false start_crio

	CNI_RESULTS_DIR=/var/lib/cni/results
	POD=$(crictl runp "$TESTDATA/sandbox_config.json")

	# CNI result is there
	# shellcheck disable=SC2010
	[[ $(ls $CNI_RESULTS_DIR | grep "$POD") != "" ]]

	# kill the sandbox
	runtime kill "$POD" KILL

	# wait for the pod to be killed
	while crictl inspectp "$POD" | jq -e '.status.state != "SANDBOX_NOTREADY"' > /dev/null; do
		echo Waiting for sandbox to be stopped
	done

	# now remove the sandbox
	crictl rmp "$POD"

	# CNI result is gone
	# shellcheck disable=SC2010
	[[ $(ls $CNI_RESULTS_DIR | grep "$POD") == "" ]]
}

@test "Clean up network if pod netns gets destroyed" {
	start_crio

	POD=$(crictl runp "$TESTDATA/sandbox_config.json")

	# remove the network namespace
	NETNS_PATH=/var/run/netns/
	NS=$(crictl inspectp "$POD" |
		jq -er '.info.runtimeSpec.linux.namespaces[] | select(.type == "network").path | sub("'$NETNS_PATH'"; "")')

	# remove network namespace
	ip netns del "$NS"

	# fake invalid netns path
	touch "$NETNS_PATH$NS"

	# be able to remove the sandbox
	crictl rmp -f "$POD"
	grep -q "Removed invalid netns path $NETNS_PATH$NS from pod sandbox" "$CRIO_LOG"
}
