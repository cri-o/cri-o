#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
	rm -f /var/lib/cni/networks/$RANDOM_CNI_NETWORK/*
}

@test "ensure correct hostname" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	run crictl exec --sync "$ctr_id" sh -c "hostname"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *"crictl_host"* ]]

	run crictl exec --sync "$ctr_id" sh -c "echo \$HOSTNAME"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *"crictl_host"* ]]

	run crictl exec --sync "$ctr_id" sh -c "cat /etc/hostname"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *"crictl_host"* ]]
}

@test "ensure correct hostname for hostnetwork:true" {
	start_crio
	python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["security_context"]["namespace_options"]["network"] = 2; obj["annotations"] = {}; obj["hostname"] = ""; json.dump(obj, sys.stdout)' \
	< "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_hostnetwork_config.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_hostnetwork_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox_hostnetwork_config.json)
	crictl start "$ctr_id"

	run crictl exec --sync "$ctr_id" sh -c "hostname"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *"$HOSTNAME"* ]]

	run crictl exec --sync "$ctr_id" sh -c "echo \$HOSTNAME"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *"$HOSTNAME"* ]]

	run crictl exec --sync "$ctr_id" sh -c "cat /etc/hostname"
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *"$HOSTNAME"* ]]
}

@test "Check for valid pod netns CIDR" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	run crictl exec --sync "$ctr_id" ip addr show dev eth0 scope global
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" = *"$POD_IPV4_CIDR_START"* ]]
	[[ "$output" = *"$POD_IPV6_CIDR_START"* ]]
}

@test "Ensure correct CNI plugin namespace/name/container-id arguments" {
	start_crio "" "prepare_plugin_test_args_network_conf"
	crictl runp "$TESTDATA"/sandbox_config.json

	# shellcheck disable=SC1091
	. "$TESTDIR"/plugin_test_args.out

	[ "$FOUND_CNI_CONTAINERID" != "redhat.test.crio" ]
	[ "$FOUND_CNI_CONTAINERID" != "podsandbox1" ]
	[ "$FOUND_K8S_POD_NAMESPACE" = "redhat.test.crio" ]
	[ "$FOUND_K8S_POD_NAME" = "podsandbox1" ]
}

@test "Connect to pod hostport from the host" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config_hostport.json)
	host_ip=$(get_host_ip)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config_hostport.json "$TESTDATA"/sandbox_config_hostport.json)
	crictl start "$ctr_id"

	run nc -w 5 "$host_ip" 4888 </dev/null
	echo "$output"
	[ "$output" = "crictl_host" ]
	[ "$status" -eq 0 ]
}

@test "Clean up network if pod sandbox fails" {
	# TODO FIXME find a way for sandbox setup to fail if manage ns is true
	CONMON_BINARY="$TESTDIR"/conmon
	cp "$(which conmon)" "$CONMON_BINARY"
	CONTAINER_MANAGE_NS_LIFECYCLE=false \
		CONTAINER_DROP_INFRA_CTR=false \
		start_crio "" "prepare_plugin_test_args_network_conf"

	# make conmon non-executable to cause the sandbox setup to fail after
	# networking has been configured
	chmod 0644 "$CONMON_BINARY"
	crictl runp "$TESTDATA"/sandbox_config.json && fail "expected runp to fail"

	# ensure that the server cleaned up sandbox networking if the sandbox
	# failed after network setup
	rm -f /var/lib/cni/networks/$RANDOM_CNI_NETWORK/last_reserved_ip*
	num_allocated=$(ls /var/lib/cni/networks/$RANDOM_CNI_NETWORK | grep -v lock | wc -l)
	[[ "${num_allocated}" == "0" ]]
}

@test "Clean up network if pod sandbox fails after plugin success" {
	start_crio "" "prepare_plugin_test_args_network_conf_malformed_result"

	crictl runp "$TESTDATA"/sandbox_config.json && fail "expected runp to fail"

	# ensure that the server cleaned up sandbox networking if the sandbox
	# failed during network setup after the CNI plugin itself succeeded
	rm -f /var/lib/cni/networks/$RANDOM_CNI_NETWORK/last_reserved_ip*
	num_allocated=$(ls /var/lib/cni/networks/$RANDOM_CNI_NETWORK | grep -v lock | wc -l)
	[[ "${num_allocated}" == "0" ]]
}
