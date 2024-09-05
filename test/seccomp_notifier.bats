#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	if ! "$CHECKSECCOMP_BINARY"; then
		skip "seccomp is not enabled"
	fi

	setup_test
}

function teardown() {
	cleanup_test
}

@test "seccomp notifier with runtime/default" {
	# Run with enabled feature set
	setup_crio
	create_runtime_with_allowed_annotation seccomp io.kubernetes.cri-o.seccompNotifierAction
	PORT=$(free_port)
	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT=$PORT start_crio_no_setup

	# Run with runtime/default
	jq '.linux.security_context.seccomp.profile_type = 0' \
		"$TESTDATA"/container_redis.json > "$TESTDIR"/container.json

	# Enable the annotation in the sandbox
	jq '.annotations += { "io.kubernetes.cri-o.seccompNotifierAction": "stop" }' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	CTR=$(crictl run "$TESTDIR"/container.json "$TESTDIR"/sandbox.json)

	for _ in 1 2 3; do
		run ! crictl exec -s "$CTR" swapoff -a
		sleep 1
	done

	sleep 6 # wait until the notifier stop the workload

	# Assert
	grep -q "Got seccomp notifier message for container ID: $CTR (syscall = swapoff)" "$CRIO_LOG"
	# Check if container exited
	crictl inspect "$CTR" | jq -e '.status.state == "CONTAINER_EXITED"'
	crictl inspect "$CTR" | jq -e '.status.reason == "seccomp killed"'
	crictl inspect "$CTR" | jq -e '.status.message == "Used forbidden syscalls: swapoff (3x)"'
	curl -sf "http://localhost:$PORT/metrics" | grep 'container_runtime_crio_containers_seccomp_notifier_count_total{name="k8s_podsandbox1-redis_podsandbox1_redhat.test.crio_redhat-test-crio_0",syscall="swapoff"} 3'
}

@test "seccomp notifier with runtime/default but not stop" {
	# Run with enabled feature set
	setup_crio
	create_runtime_with_allowed_annotation seccomp io.kubernetes.cri-o.seccompNotifierAction
	PORT=$(free_port)
	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT=$PORT start_crio_no_setup

	# Run with runtime/default
	jq '.linux.security_context.seccomp.profile_type = 0' \
		"$TESTDATA"/container_redis.json > "$TESTDIR"/container.json

	# Enable the annotation in the sandbox
	jq '.annotations += { "io.kubernetes.cri-o.seccompNotifierAction": "" }' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	CTR=$(crictl run "$TESTDIR"/container.json "$TESTDIR"/sandbox.json)

	for _ in 1 2 3; do
		run ! crictl exec -s "$CTR" swapoff -a
		sleep 1
	done

	# Assert
	grep -q "Got seccomp notifier message for container ID: $CTR (syscall = swapoff)" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e '.status.state == "CONTAINER_RUNNING"'
	curl -sf "http://localhost:$PORT/metrics" | grep 'container_runtime_crio_containers_seccomp_notifier_count_total{name="k8s_podsandbox1-redis_podsandbox1_redhat.test.crio_redhat-test-crio_0",syscall="swapoff"} 3'
}

@test "seccomp notifier with custom profile" {
	# Run with enabled feature set
	setup_crio
	create_runtime_with_allowed_annotation seccomp io.kubernetes.cri-o.seccompNotifierAction
	start_crio_no_setup

	# Run with custom profile
	sed -e 's/"chmod",//' -e 's/"fchmod",//' -e 's/"fchmodat",//g' \
		"$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/profile.json
	sed -i 's;swapon;chmod;' "$TESTDIR"/profile.json

	jq '.linux.security_context.seccomp.profile_type = 2 | .linux.security_context.seccomp.localhost_ref = "'"$TESTDIR"'/profile.json"' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/container.json

	# Enable the annotation in the sandbox
	jq '.annotations += { "io.kubernetes.cri-o.seccompNotifierAction": "stop" }' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	CTR=$(crictl run "$TESTDIR"/container.json "$TESTDIR"/sandbox.json)
	for _ in 1 2 3 4 5; do
		run ! crictl exec -s "$CTR" chmod 777 .

		run ! crictl exec -s "$CTR" swapoff -a
	done

	sleep 6 # wait until the notifier stop the workload

	# Assert
	grep -q "Got seccomp notifier message for container ID: $CTR (syscall = swapoff)" "$CRIO_LOG"
	# Check if container exited
	crictl inspect "$CTR" | jq -e '.status.state == "CONTAINER_EXITED"'
	crictl inspect "$CTR" | jq -e '.status.reason == "seccomp killed"'
	crictl inspect "$CTR" | jq -e '.status.message == "Used forbidden syscalls: swapoff (5x)"'
}

@test "seccomp notifier should not work if annotation is not allowed" {
	# Run with enabled feature set but not allowed annotation
	start_crio

	# Run with custom profile
	sed -e 's/"chmod",//' -e 's/"fchmod",//' -e 's/"fchmodat",//g' \
		"$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/profile.json
	sed -i 's;swapoff;chmod;' "$TESTDIR"/profile.json

	jq '.linux.security_context.seccomp.profile_type = 2 | .linux.security_context.seccomp.localhost_ref = "'"$TESTDIR"'/profile.json"' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR"/container.json

	# Enable the annotation in the sandbox
	jq '.annotations += { "io.kubernetes.cri-o.seccompNotifierAction": "stop" }' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	CTR=$(crictl run "$TESTDIR"/container.json "$TESTDIR"/sandbox.json)
	run ! crictl exec -s "$CTR" chmod 777 .

	# Assert
	run ! grep -q "Got seccomp notifier message for container ID: $CTR" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e '.status.state == "CONTAINER_RUNNING"'
}
