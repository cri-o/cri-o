#!/usr/bin/env bats

load helpers

function setup() {
	helptext=$(crictl runp -h)
	if [[ "$helptext" != *"cancel-timeout"* ]]; then
		skip "must have a crictl with the -T option to test CRI-O's timeout handling"
	fi
	setup_test
}

function teardown() {
	cleanup_test
}

function create_conmon() {
	timeout=$1
	cat >"$TESTDIR"/tmp_conmon <<EOF
#!/bin/sh
if [[ "\$1" != "--version" ]]; then
	sleep $timeout
fi
$CONMON_BINARY \$@
EOF
	chmod +x "$TESTDIR/tmp_conmon"
}

@test "should not clean up pod after timeout" {
	create_conmon 2s
	# need infra container so we can timeout in conmon
	CONTAINER_DROP_INFRA=false CONTAINER_CONMON="$TESTDIR/tmp_conmon" start_crio
	run crictl runp -T 3s "$TESTDATA"/sandbox_config.json
	echo "$output"
	[[ "$output" == *"context deadline exceeded"* ]]
	[ "$status" -ne 0 ]

	# allow cri-o to catch up
	sleep 2s

	# cri-o should not report any pods
	pods=$(crictl pods -q)
	[[ -z "$pods" ]]

	created_ctr_id=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q)
	[[ ! -z "$created_ctr_id" ]]

	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" == *"$created_ctr_id"* ]]
}

@test "should not clean up container after timeout" {
	create_conmon 2s
	CONTAINER_CONMON="$TESTDIR/tmp_conmon" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	run crictl create -T 2s $pod_id "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[[ "$output" == *"context deadline exceeded"* ]]
	[ "$status" -ne 0 ]

	# allow cri-o to catch up
	sleep 2s

	# cri-o should not report any pods
	ctrs=$(crictl ps -aq)
	[[ -z "$ctrs" ]]

	# cri-o should have created a container
	created_ctr_id=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q | grep -v $pod_id)
	[[ ! -z "$created_ctr_id" ]]

	run crictl create $pod_id "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]
	[[ "$output" == *"$created_ctr_id"* ]]
}

@test "should clean up pod after timeout if request changes" {
	create_conmon 2s
	# need infra container so we can timeout in conmon
	CONTAINER_DROP_INFRA=false CONTAINER_CONMON="$TESTDIR/tmp_conmon" start_crio
	run crictl runp -T 3s "$TESTDATA"/sandbox_config.json
	echo "$output"
	[[ "$output" == *"context deadline exceeded"* ]]
	[ "$status" -ne 0 ]

	# allow cri-o to catch up
	sleep 2s

	created_ctr_id=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q)
	[[ ! -z "$created_ctr_id" ]]

	# we should create a new pod and not reuse the old one
	run crictl runp <(cat "$TESTDATA"/sandbox_config.json | jq '.metadata.attempt = 2')
	[ "$status" -eq 0 ]

	[[ "$output" != *"$created_ctr_id"* ]]

	sleep 150s

	# the old, timed out container should have been removed
	run "$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q
	[[ "$output" != *"$created_ctr_id"* ]]
}

@test "should clean up container after timeout if request changes" {
	create_conmon 2s
	CONTAINER_CONMON="$TESTDIR/tmp_conmon" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	run crictl create -T 2s $pod_id "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[[ "$output" == *"context deadline exceeded"* ]]
	[ "$status" -ne 0 ]

	# allow cri-o to catch up
	sleep 2s

	# cri-o should have created a container
	created_ctr_id=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q | grep -v $pod_id)
	[[ ! -z "$created_ctr_id" ]]

	# should create a new container and not reuse the old one
	run crictl create $pod_id <(cat "$TESTDATA"/container_config.json | jq '.metadata.attempt = 2') "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]
	[[ "$output" != *"$created_ctr_id"* ]]

	sleep 150s

	# the old, timed out container should have been removed
	run "$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q
	[[ "$output" != *"$created_ctr_id"* ]]
}

@test "should clean up pod after timeout if not re-requested" {
	create_conmon 2s
	# need infra container so we can timeout in conmon
	CONTAINER_DROP_INFRA=false CONTAINER_CONMON="$TESTDIR/tmp_conmon" start_crio
	run crictl runp -T 3s "$TESTDATA"/sandbox_config.json
	echo "$output"
	[[ "$output" == *"context deadline exceeded"* ]]
	[ "$status" -ne 0 ]

	# allow cri-o to catch up and clear its state of the pod
	sleep 3m

	# cri-o should not report any pods
	pods=$(crictl pods -q)
	[[ -z "$pods" ]]

	# pod should have been cleaned up
	[[ -z $("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q) ]]

	# we should recreate the pod and not reuse the old one
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "should clean up container after timeout if not re-requested" {
	create_conmon 2s
	CONTAINER_CONMON="$TESTDIR/tmp_conmon" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	run crictl create -T 2s $pod_id "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[[ "$output" == *"context deadline exceeded"* ]]
	[ "$status" -ne 0 ]

	# allow cri-o to catch up and clear its state of the container
	sleep 150s

	# cri-o should not report any pods
	ctrs=$(crictl ps -aq)
	[[ -z "$ctrs" ]]

	# container should have been cleaned up
	[[ -z $("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q | grep -v "$pod_id") ]]

	# we should recreate the container and not reuse the old one
	run crictl create $pod_id "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
}

# this test case is paranoid, but mostly checks that we can't
# operate on a pod that's not created, and that we don't mark
# a timed out pod as created before it's re-requested
@test "should not be able to operate on a timed out pod" {
	create_conmon 2s
	# need infra container so we can timeout in conmon
	CONTAINER_DROP_INFRA=false CONTAINER_CONMON="$TESTDIR/tmp_conmon" start_crio
	run crictl runp -T 3s "$TESTDATA"/sandbox_config.json
	echo "$output"
	[[ "$output" == *"context deadline exceeded"* ]]
	[ "$status" -ne 0 ]

	# allow cri-o to catch up and clear its state of the pod
	sleep 2s

	# container should not have been cleaned up
	created_ctr_id=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q)
	[[ ! -z "$created_ctr_id" ]]

	run crictl create $created_ctr_id "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -ne 0 ]

	run crictl stopp $created_ctr_id
	echo "$output"
	[ "$status" -ne 0 ]

	run crictl inspectp $created_ctr_id
	echo "$output"
	[ "$status" -ne 0 ]
}

@test "should not be able to operate on a timed out container" {
	create_conmon 2s
	CONTAINER_CONMON="$TESTDIR/tmp_conmon" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	run crictl create -T 2s $pod_id "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[[ "$output" == *"context deadline exceeded"* ]]
	[ "$status" -ne 0 ]

	# allow cri-o to catch up
	sleep 2s

	# cri-o should have created a container
	created_ctr_id=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list -q | grep -v $pod_id)
	[[ ! -z "$created_ctr_id" ]]

	run crictl start $created_ctr_id
	echo "$output"
	[ "$status" -ne 0 ]

	run crictl exec $created_ctr_id ls
	echo "$output"
	[ "$status" -ne 0 ]

	run crictl exec --sync $created_ctr_id ls
	echo "$output"
	[ "$status" -ne 0 ]

	run crictl inspect $created_ctr_id
	echo "$output"
	[ "$status" -ne 0 ]
}

# TODO:
# test a restore cleans out containers that aren't yet created
