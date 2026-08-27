#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	sboxconfig="$TESTDIR/sandbox_config.json"
	ctrconfig="$TESTDIR/container_config.json"
}

function teardown() {
	cleanup_test
}

@test "internal annotation in pod annotations should be dropped" {
	start_crio

	jq '.annotations."io.kubernetes.cri-o.SandboxID" = "injected-value"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$sboxconfig")
	crictl start "$ctr_id"

	ann=$(crictl inspect "$ctr_id" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.cri-o.SandboxID"]')
	[[ "$ann" != "injected-value" ]]
}

@test "internal annotation in pod labels should be dropped" {
	start_crio

	jq '.labels."io.kubernetes.cri-o.MountPoint" = "injected-value"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$sboxconfig")
	crictl start "$ctr_id"

	ann=$(crictl inspect "$ctr_id" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.cri-o.MountPoint"]')
	[[ "$ann" != "injected-value" ]]
}

@test "internal annotation in container annotations should be dropped" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '.annotations."io.kubernetes.cri-o.ContainerID" = "injected-value"' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl create "$pod_id" "$ctrconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	ann=$(crictl inspect "$ctr_id" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.cri-o.ContainerID"]')
	[[ "$ann" != "injected-value" ]]
}

@test "internal annotation in container labels should be dropped" {
	start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '.labels."io.kubernetes.cri-o.Name" = "injected-value"' \
		"$TESTDATA"/container_sleep.json > "$ctrconfig"

	ctr_id=$(crictl create "$pod_id" "$ctrconfig" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	ann=$(crictl inspect "$ctr_id" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.cri-o.Name"]')
	[[ "$ann" != "injected-value" ]]
}

@test "non-internal annotations should pass through" {
	start_crio

	jq '.annotations."com.example.safe-annotation" = "safe-value"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$sboxconfig")
	crictl start "$ctr_id"

	ann=$(crictl inspect "$ctr_id" | jq -r '.info.runtimeSpec.annotations["com.example.safe-annotation"]')
	[[ "$ann" == "safe-value" ]]
}

@test "internal annotations survive crio restart with original values" {
	start_crio

	jq '  .annotations."io.kubernetes.cri-o.SandboxID" = "injected-sandbox"
		| .labels."io.kubernetes.cri-o.MountPoint" = "injected-mount"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$sboxconfig")
	crictl start "$ctr_id"

	sandbox_before=$(crictl inspect "$ctr_id" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.cri-o.SandboxID"]')
	[[ "$sandbox_before" != "injected-sandbox" ]]
	[[ "$sandbox_before" != "null" ]]

	restart_crio

	sandbox_after=$(crictl inspect "$ctr_id" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.cri-o.SandboxID"]')
	[[ "$sandbox_after" == "$sandbox_before" ]]

	mount_after=$(crictl inspect "$ctr_id" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.cri-o.MountPoint"]')
	[[ "$mount_after" != "injected-mount" ]]
}

@test "allowed v1 feature annotation should not be dropped" {
	create_workload_with_allowed_annotation "io.kubernetes.cri-o.DisableFIPS"
	start_crio

	jq '.annotations."io.kubernetes.cri-o.DisableFIPS" = "true"' \
		"$TESTDATA"/sandbox_config.json > "$sboxconfig"

	pod_id=$(crictl runp "$sboxconfig")

	ann=$(crictl inspectp "$pod_id" | jq -r '.info.runtimeSpec.annotations["io.kubernetes.cri-o.DisableFIPS"]')
	[[ "$ann" == "true" ]]

	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}
