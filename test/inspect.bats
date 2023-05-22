#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "info inspect" {
	start_crio
	out=$(echo -e "GET /info HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	echo "$out"
	[[ "$out" == *"\"cgroup_driver\":\"$CONTAINER_CGROUP_MANAGER\""* ]]
	[[ "$out" == *"\"storage_root\":\"$TESTDIR/crio\""* ]]
}

@test "ctr inspect" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)

	out=$(echo -e "GET /containers/$ctr_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	[[ "$out" == *"\"sandbox\":\"$pod_id\""* ]]
	[[ "$out" == *"\"image\":\"quay.io/crio/fedora-crio-ci:latest\""* ]]
	[[ "$out" == *"\"image_ref\":\"$REDIS_IMAGEREF\""* ]]

	output=$(crictl inspect --output json "$ctr_id")
	[[ "$output" == *"\"id\": \"$ctr_id\""* ]]
	[[ "$output" == *"\"image\": \"quay.io/crio/fedora-crio-ci:latest\""* ]]
	[[ "$output" == *"\"imageRef\": \"$REDIS_IMAGEREF\""* ]]

	output=$(crictl inspectp --output json "$pod_id")

	ipv4=$(pod_ip -4 "$ctr_id")
	ipv6=$(pod_ip -6 "$ctr_id")
	[[ "$out" == *"\"ip_addresses\":[\"$ipv4\",\"$ipv6\"]"* ]]
	[[ "$output" == *"\"ip\": \"$ipv4\""* ]]

	# TODO: add some other check based on the json below:
	#
	# {"name":"k8s_container1_podsandbox1_redhat.test.crio_redhat-test-crio_1","pid":27477,"image":"redis:alpine","created_time":1505223601111546169,"labels":{"batch":"no","type":"small"},"annotations":{"daemon":"crio","owner":"dragon"},"log_path":"/var/log/crio/pods/297d014ba2c54236779da0c2f80dfba45dc31b106e4cd126a1c3c6d78edc2201/81567e9573ea798d6494c9aab156103ee91b72180fd3841a7c24d2ca39886ba2.log","root":"/tmp/tmp.0bkjphWudF/crio/overlay/d7cfc1de83cab9f377a4a1542427d2a019e85a70c1c660a9e6cf9e254df68873/merged","sandbox":"297d014ba2c54236779da0c2f80dfba45dc31b106e4cd126a1c3c6d78edc2201","ip_addresses":"[10.88.9.153]"}
}

@test "pod inspect when dropping infra" {
	CONTAINER_DROP_INFRA_CTR=true start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# before there's a running container, the PID will be 0
	out=$(echo -e "GET /containers/$pod_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	[[ "$out" == *"\"sandbox\":\"$pod_id\""* ]]
	[[ "$out" == *"\"pid\":0"* ]]
	[[ "$out" == *"\"root\":\"$TESTDIR/crio/overlay/"* ]]

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	ctr_pid=$(runtime state "$ctr_id" | jq .pid)

	# after there's a running container, the PID will be the first running container PID
	out=$(echo -e "GET /containers/$pod_id HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	[[ "$out" == *"\"sandbox\":\"$pod_id\""* ]]
	[[ "$out" == *"\"pid\":$ctr_pid"* ]]
}

@test "ctr inspect not found" {
	start_crio
	out=$(echo -e "GET /containers/notexists HTTP/1.1\r\nHost: crio\r\n" | socat - UNIX-CONNECT:"$CRIO_SOCKET")
	[[ "$out" == *"can't find the container with id notexists"* ]]
}
