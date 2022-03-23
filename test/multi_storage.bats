#!/usr/bin/env bats

TESTIMAGE="quay.io/crio/redis:alpine"

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

# For this test series we always need the --with-pull image because the images are not pre-pulled for the additional storage

@test "pull image for the additional storage" {
	start_crio
	pod_id=$(crictl runp --runtime="${RUNTIME_HANDLER}" "$TESTDATA"/sandbox_config.json)
	crictl pull --pod-config "$TESTDATA"/sandbox_config.json "$TESTIMAGE"
}

@test "pull image at container creation" {
	start_crio
	pod_id=$(crictl runp --runtime="${RUNTIME_HANDLER}" "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create --with-pull "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
}

@test "remove image" {
	start_crio
	crictl pull "$TESTIMAGE"
	pod_id=$(crictl runp --runtime="${RUNTIME_HANDLER}" "$TESTDATA"/sandbox_config.json)
	crictl pull --pod-config "$TESTDATA"/sandbox_config.json "$TESTIMAGE"
	crictl rmi "$TESTIMAGE"
	output=$(crictl images | grep "$TESTIMAGE" || true)
	[ "$output" = "" ]
}

@test "remove image pull for the additional storage" {
	start_crio
	pod_id=$(crictl runp --runtime="${RUNTIME_HANDLER}" "$TESTDATA"/sandbox_config.json)
	crictl pull --pod-config "$TESTDATA"/sandbox_config.json "$TESTIMAGE"
	crictl rmi "$TESTIMAGE"
	output=$(crictl images | grep "$TESTIMAGE" || true)
	[ "$output" = "" ]
}

@test "run 2 containers with the same image but different runtime class and storage" {
	start_crio
	pod_config="$TESTDATA"/sandbox_config.json
	pod1_config="$TESTDIR"/sandbox1_config.json
	pod2_config="$TESTDIR"/sandbox2_config.json

	jq '	  .metadata.name = "podsandbox1"
		| .metadata.uid = "redhat-test-crio-1"
		| .labels.group = "test"
		| .labels.name = "podsandbox1"
		| .labels.version = "v1.0.0"' \
		"$TESTDATA"/sandbox_config.json > "$pod1_config"
	pod1_id=$(crictl runp "$pod1_config")
	ctr1_id=$(crictl create "$pod1_id" "$TESTDATA"/container_redis.json "$pod_config")
	crictl start "$ctr1_id"

	jq '	  .metadata.name = "podsandbox2"
		| .metadata.uid = "redhat-test-crio-2"
		| .labels.group = "test"
		| .labels.name = "podsandbox2"
		| .labels.version = "v1.0.0"' \
		"$TESTDATA"/sandbox_config.json > "$pod2_config"
	pod2_id=$(crictl runp --runtime="${RUNTIME_HANDLER}" "$pod2_config")
	ctr2_id=$(crictl create --with-pull "$pod2_id" "$TESTDATA"/container_redis.json "$pod2_config")
	crictl start "$ctr2_id"
}
