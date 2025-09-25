#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

IMAGE=quay.io/crio/fedora-crio-ci:latest
IMAGE_SHA256=7aa25047c59c6c2e20b839f5b9820e3702dade4a97154d4df1f5cd1a9b44cfde
NAMESPACE=default

function setup() {
	setup_test
	CONTAINER_NAMESPACED_AUTH_DIR="$TESTDIR/auth" start_crio
}

function teardown() {
	cleanup_test
}

@test "should use and remove a namespaced auth file if available" {
	echo '{}' > "$TESTDIR/auth/$NAMESPACE-$IMAGE_SHA256.json"
	jq '.metadata.namespace = "'$NAMESPACE'"' "$TESTDATA/sandbox_config.json" > "$TESTDIR/sb.json"

	crictl pull --pod-config "$TESTDIR/sb.json" $IMAGE

	grep -q "Looking for namespaced auth JSON file in: .*$IMAGE_SHA256" "$CRIO_LOG"
	grep -q "Using auth file for namespace $NAMESPACE" "$CRIO_LOG"
	run ! test -f "$(sed -n 's;.*Removed temp auth file: \(.*\.json\).*;\1;p' "$CRIO_LOG")"
	[[ $(find "$TESTDIR/auth" -type f) == "" ]]
}

@test "should fail with invalid credentials" {
	echo '{"auths":{"quay.io":{"auth": "bXl1c2VyOm15cGFzc3dvcmQ="}}}' > "$TESTDIR/auth/$NAMESPACE-$IMAGE_SHA256.json"
	jq '.metadata.namespace = "'$NAMESPACE'"' "$TESTDATA/sandbox_config.json" > "$TESTDIR/sb.json"

	run ! crictl pull --pod-config "$TESTDIR/sb.json" $IMAGE

	grep -q "Looking for namespaced auth JSON file in: .*$IMAGE_SHA256" "$CRIO_LOG"
	grep -q "Using auth file for namespace $NAMESPACE" "$CRIO_LOG"
	grep -q "invalid username/password: unauthorized" "$CRIO_LOG"
	run ! test -f "$(sed -n 's;.*Removed temp auth file: \(.*\.json\).*;\1;p' "$CRIO_LOG")"
	[[ $(find "$TESTDIR/auth" -type f) == "" ]]
}

@test "should not use auth file if namespace does not match" {
	AUTHFILE="$TESTDIR/auth/$NAMESPACE-$IMAGE_SHA256.json"
	echo '{}' > "$AUTHFILE"
	crictl pull --pod-config "$TESTDATA/sandbox_config.json" $IMAGE

	grep -q "Looking for namespaced auth JSON file in: .*$IMAGE_SHA256" "$CRIO_LOG"
	grep -vq "Using auth file for namespace $NAMESPACE" "$CRIO_LOG"
	grep -vq "Removed temp auth file:" "$CRIO_LOG"
	test -f "$AUTHFILE"
}

@test "should fail to pull if auth file is malformed" {
	echo wrong-content > "$TESTDIR/auth/$NAMESPACE-$IMAGE_SHA256.json"
	jq '.metadata.namespace = "'$NAMESPACE'"' "$TESTDATA/sandbox_config.json" > "$TESTDIR/sb.json"

	run ! crictl pull --pod-config "$TESTDIR/sb.json" $IMAGE

	grep -q "Looking for namespaced auth JSON file in: .*$IMAGE_SHA256" "$CRIO_LOG"
	grep -q "Using auth file for namespace $NAMESPACE" "$CRIO_LOG"
	grep -q "invalid character 'w' looking for beginning of value" "$CRIO_LOG"
	run ! test -f "$(sed -n 's;.*Removed temp auth file: \(.*\.json\).*;\1;p' "$CRIO_LOG")"
	[[ $(find "$TESTDIR/auth" -type f) == "" ]]
}
