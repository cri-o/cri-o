#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "ensure correct hostname" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

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

	pod_id=$(crictl runp "$TESTDIR"/sandbox_hostnetwork.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox_hostnetwork.json)
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" sh -c "hostname")
	[[ "$output" == *"$HOSTNAME"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "echo \$HOSTNAME")
	[[ "$output" == *"$HOSTNAME"* ]]

	output=$(crictl exec --sync "$ctr_id" sh -c "cat /etc/hostname")
	[[ "$output" == *"$HOSTNAME"* ]]
}

@test "Check for valid pod netns CIDR" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

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
}

@test "Connect to pod hostport from the host" {
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
	jq '	  .image.image = "quay.io/crio/busybox:latest"
		| .command = [ "/bin/nc", "-ll", "-p", "80", "-e", "/bin/hostname" ]' \
		"$TESTDATA"/container_config.json > "$ctr_config"

	pod_id=$(crictl runp "$pod_config")
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

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
	# TODO FIXME find a way for sandbox setup to fail if manage ns is true
	CONMON_BINARY="$TESTDIR"/conmon
	cp "$(command -v conmon)" "$CONMON_BINARY"
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}"
	CONTAINER_MANAGE_NS_LIFECYCLE=false \
		CONTAINER_DROP_INFRA_CTR=false \
		start_crio

	# make conmon non-executable to cause the sandbox setup to fail after
	# networking has been configured
	chmod 0644 "$CONMON_BINARY"
	crictl runp "$TESTDATA"/sandbox_config.json && fail "expected runp to fail"

	check_networking
}

@test "Clean up network if pod sandbox fails after plugin success" {
	CNI_DEFAULT_NETWORK="crio-${TESTDIR: -10}"
	CNI_TYPE="cni_plugin_helper.bash" setup_crio
	echo "DEBUG_ARGS=malformed-result" > "$TESTDIR"/cni_plugin_helper_input.env
	start_crio_no_setup
	check_images

	crictl runp "$TESTDATA"/sandbox_config.json && fail "expected runp to fail"

	check_networking
}
