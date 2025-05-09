#!/usr/bin/env bats

load helpers

function setup() {
  if [[ $RUNTIME_TYPE != oci ]]; then
    skip "not using conmonrs"
  fi
	setup_test
}

function teardown() {
	cleanup_test
}

@test "cri-o logs the error when conmon exits" {
	start_crio

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	conmon_pid=$(jq '.containerMonitorProcess.pid'< "$TESTDIR/crio/overlay-containers/$ctr_id/userdata/state.json")

	kill -9 "$conmon_pid"
	sleep 10
	wait_for_log "Monitor for container"
}

@test "cri-o doesn't log the error when conmon doesn't exit" {
	start_crio

	crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json
	sleep 10
	run ! wait_for_log "Monitor for container"
}

@test "cri-o stops monitoring when the container stops" {
	start_crio

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl stop "$ctr_id"
	wait_for_log "DeleteMonitoringProcess"
}

@test "cri-o continues to monitor when cri-o restarts" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	conmon_pid=$(jq '.containerMonitorProcess.pid'< "$TESTDIR/crio/overlay-containers/$ctr_id/userdata/state.json")

  restart_crio
	kill -9 "$conmon_pid"
	sleep 10
	wait_for_log "Monitor for container"
}
