#!/usr/bin/env bats

load helpers

function setup() {
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "test needs conmon, not conmon-rs"
	fi
	# do not use the crictl() wrapper function here: we need to test the crictl
	# features with no additional arg.
	if ! "$CRICTL_BINARY" runp -h | grep -q "cancel-timeout"; then
		skip "must have a crictl with the -T option to test CRI-O's timeout handling"
	fi
	setup_test

	create_conmon 3s
	CANCEL_TIMEOUT="3s"
}

function teardown() {
	cleanup_test
}

function create_conmon() {
	local timeout=$1

	cat > "$TESTDIR"/tmp_conmon << EOF
#!/usr/bin/env bash
if [[ "\$1" != "--version" ]]; then
	sleep $timeout
fi
$CONMON_BINARY \$@
EOF
	chmod +x "$TESTDIR/tmp_conmon"

	export CONTAINER_CONMON="$TESTDIR/tmp_conmon"
}

function create_pinns() {
	local timeout=$1

	cat > "$TESTDIR"/tmp_pinns << EOF
#!/usr/bin/env bash
if [[ "\$1" != "--version" ]]; then
    echo "Delaying pinns by $timeout"
	sleep $timeout
fi
$PINNS_BINARY_PATH \$@
EOF
	chmod +x "$TESTDIR/tmp_pinns"

	export CONTAINER_PINNS_PATH="$TESTDIR/tmp_pinns"
}

# Allow cri-o to catch up. The sleep here should be less than
# resourcestore.sleepTimeBeforeCleanup but enough for cri-o to
# finish processing cancelled crictl create/runp.
function wait_create() {
	sleep 30s
}

# Allow cri-o to catch up and clean the state of a pod/container.
# The sleep here should be > 2 * resourcestore.sleepTimeBeforeCleanup.
function wait_clean() {
	sleep 150s
}

function verify_context_deadline_exceeded() {
	[[ "$1" == *"context deadline exceeded"* || "$1" == *"DeadlineExceeded"* ]]
}

@test "should not clean up pod after timeout" {
	create_pinns "$CANCEL_TIMEOUT"

	# need infra container so runp can timeout in conmon
	CONTAINER_DROP_INFRA_CTR=false start_crio
	run ! crictl runp -T "$CANCEL_TIMEOUT" "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"

	wait_create

	# cri-o should not report any pods
	pods=$(crictl pods -q)
	[[ -z "$pods" ]]

	created_ctr_id=$(runtime list -q)
	[ -n "$created_ctr_id" ]

	output=$(crictl runp "$TESTDATA"/sandbox_config.json)
	[[ "$output" == "$created_ctr_id" ]]
}

@test "emit metric when sandbox is re-requested" {
	create_pinns "$CANCEL_TIMEOUT"

	# need infra container so runp can timeout in conmon
	PORT=$(free_port)
	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT="$PORT" CONTAINER_DROP_INFRA_CTR=false start_crio
	run crictl runp -T "$CANCEL_TIMEOUT" "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"
	[ "$status" -ne 0 ]

	run ! crictl runp -T "$CANCEL_TIMEOUT" "$TESTDATA"/sandbox_config.json

	# allow metric to be populated
	METRIC=$(curl -sf "http://localhost:$PORT/metrics" | grep 'crio_resources_stalled_at_stage{')
	# the test races against itself, so we don't know the exact stage that will be registered. It should be one of them.
	[[ "$METRIC" == 'container_runtime_crio_resources_stalled_at_stage{stage="sandbox '* ]]
}

@test "should not clean up container after timeout" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	run ! crictl create -T "$CANCEL_TIMEOUT" "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"

	wait_create

	# cri-o should not report any containers
	ctrs=$(crictl ps -aq)
	[[ -z "$ctrs" ]]

	# cri-o should have created a container
	created_ctr_id=$(runtime list -q | grep -v "$pod_id")
	[ -n "$created_ctr_id" ]

	output=$(crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json)
	[[ "$output" == "$created_ctr_id" ]]
}

@test "should clean up pod after timeout if request changes" {
	create_pinns "$CANCEL_TIMEOUT"

	# need infra container so runp can timeout in conmon
	CONTAINER_DROP_INFRA_CTR=false start_crio
	run ! crictl runp -T "$CANCEL_TIMEOUT" "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"

	wait_create

	created_ctr_id=$(runtime list -q)
	[ -n "$created_ctr_id" ]

	# we should create a new pod and not reuse the old one
	output=$(crictl runp <(jq '.metadata.attempt = 2' "$TESTDATA"/sandbox_config.json))
	[[ "$output" != "$created_ctr_id" ]]

	wait_clean

	# the old, timed out container should have been removed
	[[ "$(runtime list -q)" != *"$created_ctr_id"* ]]
}

@test "should clean up container after timeout if request changes" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	run ! crictl create -T "$CANCEL_TIMEOUT" "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"

	wait_create

	# cri-o should have created a container
	created_ctr_id=$(runtime list -q | grep -v "$pod_id")
	[ -n "$created_ctr_id" ]

	# should create a new container and not reuse the old one
	output=$(crictl create "$pod_id" <(jq '.metadata.attempt = 2' "$TESTDATA"/container_config.json) "$TESTDATA"/sandbox_config.json)
	[[ "$output" != "$created_ctr_id" ]]

	wait_clean

	# the old, timed out container should have been removed
	[[ "$(runtime list -q)" != *"$created_ctr_id"* ]]
}

@test "should clean up pod after timeout if not re-requested" {
	create_pinns "$CANCEL_TIMEOUT"

	# need infra container so runp can timeout in conmon
	CONTAINER_DROP_INFRA_CTR=false start_crio
	run ! crictl runp -T "$CANCEL_TIMEOUT" "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"

	wait_clean

	# cri-o should not report any pods
	pods=$(crictl pods -q)
	[[ -z "$pods" ]]

	# pod should have been cleaned up
	[[ -z $(runtime list -q) ]]

	# we should recreate the pod and not reuse the old one
	crictl runp "$TESTDATA"/sandbox_config.json
}

@test "should not wait for actual duplicate pod request" {
	start_crio
	pod_1=$(crictl runp "$TESTDATA"/sandbox_config.json)
	SECONDS=0
	pod_2=$(crictl runp "$TESTDATA"/sandbox_config.json)
	[[ "$SECONDS" -lt 240 ]]
	[[ "$pod_1" == "$pod_2" ]]
}

@test "should clean up container after timeout if not re-requested" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	run ! crictl create -T "$CANCEL_TIMEOUT" "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"

	wait_clean

	# cri-o should not report any containers
	ctrs=$(crictl ps -aq)
	[[ -z "$ctrs" ]]

	# container should have been cleaned up
	[[ "$(runtime list -q)" != *"$pod_id"* ]]

	# we should recreate the container and not reuse the old one
	crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
}

@test "emit metric when container is re-requested" {
	PORT=$(free_port)
	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT="$PORT" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	run crictl create -T "$CANCEL_TIMEOUT" "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"
	[ "$status" -ne 0 ]

	run ! crictl create -T "$CANCEL_TIMEOUT" "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json

	# allow metric to be populated
	METRIC=$(curl -sf "http://localhost:$PORT/metrics" | grep 'crio_resources_stalled_at_stage{')
	# the test races against itself, so we don't know the exact stage that will be registered. It should be one of them.
	[[ "$METRIC" == 'container_runtime_crio_resources_stalled_at_stage{stage="container '* ]]
}

# this test case is paranoid, but mostly checks that we can't
# operate on a pod that's not created, and that we don't mark
# a timed out pod as created before it's re-requested
@test "should not be able to operate on a timed out pod" {
	create_pinns "$CANCEL_TIMEOUT"

	# need infra container so runp can timeout in conmon
	CONTAINER_DROP_INFRA_CTR=false start_crio
	run ! crictl runp -T "$CANCEL_TIMEOUT" "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"

	wait_create

	# container should not have been cleaned up
	created_ctr_id=$(runtime list -q)
	[ -n "$created_ctr_id" ]

	run ! crictl create "$created_ctr_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	run ! crictl stopp "$created_ctr_id"
	run ! crictl inspectp "$created_ctr_id"
}

@test "should not be able to operate on a timed out container" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	run ! crictl create -T "$CANCEL_TIMEOUT" "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	verify_context_deadline_exceeded "$output"

	wait_create

	# cri-o should have created a container
	created_ctr_id=$(runtime list -q | grep -v "$pod_id")
	[ -n "$created_ctr_id" ]

	run ! crictl start "$created_ctr_id"
	run ! crictl exec "$created_ctr_id" ls
	run ! crictl exec --sync "$created_ctr_id" ls
	run ! crictl inspect "$created_ctr_id"
}

@test "should not wait for actual duplicate container request" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	SECONDS=0
	crictl create "$pod_id" "$TESTDATA"/container_config.json "$TESTDATA"/sandbox_config.json
	[[ "$SECONDS" -lt 240 ]]
}
