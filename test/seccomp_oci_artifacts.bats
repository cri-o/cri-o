#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	if ! "$CHECKSECCOMP_BINARY"; then
		skip "seccomp is not enabled"
	fi

	export CONTAINER_SECCOMP_USE_DEFAULT_WHEN_EMPTY=false
	setup_test
}

function teardown() {
	cleanup_test
}

ARTIFACT_IMAGE_WITH_ANNOTATION=quay.io/crio/nginx-seccomp:generic
ARTIFACT_IMAGE_WITH_POD_ANNOTATION=quay.io/crio/nginx-seccomp:pod
ARTIFACT_IMAGE_WITH_CONTAINER_ANNOTATION=quay.io/crio/nginx-seccomp:container
ARTIFACT_IMAGE=quay.io/crio/seccomp:v1
CONTAINER_NAME=container1
ANNOTATION=seccomp-profile.kubernetes.cri-o.io
POD_ANNOTATION=seccomp-profile.kubernetes.cri-o.io/POD
TEST_SYSCALL=OCI_ARTIFACT_TEST

@test "seccomp OCI artifact with image annotation without suffix" {
	# Run with enabled feature set
	create_runtime_with_allowed_annotation seccomp $ANNOTATION
	start_crio

	jq '.image.image = "'$ARTIFACT_IMAGE_WITH_ANNOTATION'"' \
		"$TESTDATA/container_config.json" > "$TESTDIR/container.json"

	crictl pull $ARTIFACT_IMAGE_WITH_ANNOTATION
	CTR=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# Assert
	grep -q "Found image specific seccomp profile annotation: $ANNOTATION=$ARTIFACT_IMAGE" "$CRIO_LOG"
	grep -q "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL
}

@test "seccomp OCI artifact with image annotation for pod" {
	# Run with enabled feature set
	create_runtime_with_allowed_annotation seccomp $ANNOTATION
	start_crio

	jq '.image.image = "'$ARTIFACT_IMAGE_WITH_POD_ANNOTATION'"' \
		"$TESTDATA/container_config.json" > "$TESTDIR/container.json"

	crictl pull $ARTIFACT_IMAGE_WITH_POD_ANNOTATION
	CTR=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# Assert
	grep -q "Found image specific seccomp profile annotation for pod: $ANNOTATION=$ARTIFACT_IMAGE" "$CRIO_LOG"
	grep -q "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL
}

@test "seccomp OCI artifact with image annotation for container" {
	# Run with enabled feature set
	create_runtime_with_allowed_annotation seccomp $ANNOTATION
	start_crio

	jq '.image.image = "'$ARTIFACT_IMAGE_WITH_CONTAINER_ANNOTATION'"' \
		"$TESTDATA/container_config.json" > "$TESTDIR/container.json"

	crictl pull $ARTIFACT_IMAGE_WITH_CONTAINER_ANNOTATION
	CTR=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# Assert
	grep -q "Found image specific seccomp profile annotation for container $CONTAINER_NAME: $ANNOTATION=$ARTIFACT_IMAGE" "$CRIO_LOG"
	grep -q "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL
}

@test "seccomp OCI artifact with image annotation but not allowed annotation on runtime config" {
	start_crio

	jq '.image.image = "'$ARTIFACT_IMAGE_WITH_POD_ANNOTATION'"' \
		"$TESTDATA/container_config.json" > "$TESTDIR/container.json"

	crictl pull $ARTIFACT_IMAGE_WITH_POD_ANNOTATION
	CTR=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# Assert
	grep -vq "Found image specific seccomp profile annotation: $ANNOTATION=$ARTIFACT_IMAGE" "$CRIO_LOG"
	grep -vq "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e '.info.runtimeSpec.linux.seccomp == null'
}

@test "seccomp OCI artifact with image annotation and profile set to unconfined" {
	# Run with enabled feature set
	create_runtime_with_allowed_annotation seccomp $ANNOTATION
	start_crio

	jq '.image.image = "'$ARTIFACT_IMAGE_WITH_POD_ANNOTATION'"
        | .linux.security_context.seccomp.profile_type = 1' \
		"$TESTDATA/container_config.json" > "$TESTDIR/container.json"

	crictl pull $ARTIFACT_IMAGE_WITH_POD_ANNOTATION
	CTR=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# Assert
	grep -q "Found image specific seccomp profile annotation for pod: $ANNOTATION=$ARTIFACT_IMAGE" "$CRIO_LOG"
	grep -q "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL
}

@test "seccomp OCI artifact with image annotation but set runtime default profile with higher priority" {
	# Run with enabled feature set
	create_runtime_with_allowed_annotation seccomp $ANNOTATION
	start_crio

	jq '.image.image = "'$ARTIFACT_IMAGE_WITH_POD_ANNOTATION'"
        | .linux.security_context.seccomp.profile_type = 0' \
		"$TESTDATA/container_config.json" > "$TESTDIR/container.json"

	crictl pull $ARTIFACT_IMAGE_WITH_POD_ANNOTATION
	CTR=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# Assert
	grep -vq "Found image specific seccomp profile annotation for pod: $ANNOTATION=$ARTIFACT_IMAGE" "$CRIO_LOG"
	grep -vq "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -vq $TEST_SYSCALL
}

@test "seccomp OCI artifact with image annotation but set localhost profile with higher priority" {
	# Run with enabled feature set
	create_runtime_with_allowed_annotation seccomp $ANNOTATION
	start_crio

	sed -e 's/"chmod",//' -e 's/"fchmod",//' -e 's/"fchmodat",//g' \
		"$CONTAINER_SECCOMP_PROFILE" > "$TESTDIR"/profile.json

	jq '.image.image = "'$ARTIFACT_IMAGE_WITH_POD_ANNOTATION'"
        | .linux.security_context.seccomp.profile_type = 2
        | .linux.security_context.seccomp.localhost_ref = "'"$TESTDIR"'/profile.json"' \
		"$TESTDATA/container_config.json" > "$TESTDIR/container.json"

	crictl pull $ARTIFACT_IMAGE_WITH_POD_ANNOTATION
	CTR=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# Assert
	grep -vq "Found image specific seccomp profile annotation for pod: $ANNOTATION=$ARTIFACT_IMAGE" "$CRIO_LOG"
	grep -vq "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -vq $TEST_SYSCALL
}

@test "seccomp OCI artifact with pod annotation" {
	# Run with enabled feature set
	create_runtime_with_allowed_annotation seccomp $ANNOTATION
	start_crio

	jq '.annotations += { "'$POD_ANNOTATION'": "'$ARTIFACT_IMAGE'" }' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	CTR=$(crictl run "$TESTDATA/container_config.json" "$TESTDIR/sandbox.json")

	# Assert
	grep -q "Found pod specific seccomp profile annotation: $ANNOTATION=$ARTIFACT_IMAGE" "$CRIO_LOG"
	grep -q "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL
}

@test "seccomp OCI artifact with container annotation" {
	# Run with enabled feature set
	create_runtime_with_allowed_annotation seccomp $ANNOTATION
	start_crio

	jq '.annotations += { "'$ANNOTATION'/'$CONTAINER_NAME'": "'$ARTIFACT_IMAGE'" }' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	CTR=$(crictl run "$TESTDATA/container_config.json" "$TESTDIR/sandbox.json")

	# Assert
	grep -q "Found container specific seccomp profile annotation: $ANNOTATION/$CONTAINER_NAME=$ARTIFACT_IMAGE" "$CRIO_LOG"
	grep -q "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e .info.runtimeSpec.linux.seccomp | grep -q $TEST_SYSCALL
}

@test "seccomp OCI artifact with bogus annotation" {
	# Run with enabled feature set
	create_runtime_with_allowed_annotation seccomp $ANNOTATION
	start_crio

	jq '.annotations += { "'$ANNOTATION'/container2": "'$ARTIFACT_IMAGE'" }' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	CTR=$(crictl run "$TESTDATA/container_config.json" "$TESTDIR/sandbox.json")

	# Assert
	grep -vq "Found container specific seccomp profile annotation" "$CRIO_LOG"
	grep -vq "Retrieved OCI artifact seccomp profile" "$CRIO_LOG"
	crictl inspect "$CTR" | jq -e '.info.runtimeSpec.linux.seccomp == null'
}
