#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

ARTIFACT_IMAGE=quay.io/crio/seccomp:v2

@test "should be able to pull and list an OCI artifact" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Should get listed as filtered artifact
	crictl images -q $ARTIFACT_IMAGE
	[ "$output" != "" ]

	# Should be available on the whole list
	crictl images | grep -qE 'quay.io/crio/seccomp.*v2'
}

@test "should be able to inspect an OCI artifact" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	crictl inspecti $ARTIFACT_IMAGE |
		jq -e '
		(.status.pinned == true) and
		(.status.repoDigests | length == 1) and
		(.status.repoTags | length == 1) and
		(.status.size != "0")'
}

@test "should be able to remove an OCI artifact" {
	start_crio
	crictl pull $ARTIFACT_IMAGE
	crictl rmi $ARTIFACT_IMAGE

	[ "$(crictl images -q $ARTIFACT_IMAGE | wc -l)" == 0 ]
}
