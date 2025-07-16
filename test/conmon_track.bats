#!/usr/bin/env bats

load helpers

function setup() {
	if [[ $RUNTIME_TYPE == vm ]]; then
		skip "using vm"
	fi
	setup_test
}

function teardown() {
	cleanup_test
}

@test "cri-o logs the error when conmon exits" {
	port=$(free_port)
	CONTAINER_ENABLE_METRICS=true \
		CONTAINER_METRICS_PORT=$port \
		start_crio

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	name=$(jq '.info.runtimeSpec.annotations["io.kubernetes.cri-o.Name"]' <(crictl inspect "$ctr_id"))
	conmon_pid=$(jq '.containerMonitorProcess.pid' "$TESTDIR/crio/overlay-containers/$ctr_id/userdata/state.json")
	kill -9 "$conmon_pid"
	sleep 10
	wait_for_log "Conmon for container $ctr_id"
	grep "containers_stopped_monitor_count{name=$name}" <(curl -sfk "http://localhost:$port/metrics")
}

@test "cri-o doesn't log the error when conmon doesn't exit" {
	port=$(free_port)
	CONTAINER_ENABLE_METRICS=true \
		CONTAINER_METRICS_PORT=$port \
		start_crio

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	name=$(jq '.info.runtimeSpec.annotations["io.kubernetes.cri-o.Name"]' <(crictl inspect "$ctr_id"))
	sleep 10
	run ! wait_for_log "Conmon for container $ctr_id"
	run ! grep "containers_stopped_monitor_count{name=$name}" <(curl -sfk "http://localhost:$port/metrics")
}

@test "cri-o doesn't monitor when the container stopped" {
	port=$(free_port)
	CONTAINER_ENABLE_METRICS=true \
		CONTAINER_METRICS_PORT=$port \
		start_crio

	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	name=$(jq '.info.runtimeSpec.annotations["io.kubernetes.cri-o.Name"]' <(crictl inspect "$ctr_id"))
	crictl stop "$ctr_id"
	sleep 10
	run ! wait_for_log "Conmon for container $ctr_id"
	run ! grep "containers_stopped_monitor_count{name=$name}" <(curl -sfk "http://localhost:$port/metrics")
}

@test "cri-o continues to monitor when cri-o restarts" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	conmon_pid=$(jq '.containerMonitorProcess.pid' "$TESTDIR/crio/overlay-containers/$ctr_id/userdata/state.json")

	restart_crio
	run ! wait_for_log "Failed to load conmon process"
	kill -9 "$conmon_pid"
	sleep 10
	wait_for_log "Conmon for container $ctr_id"
}

@test "cri-o doesn't monitor conmon when conmon is stopped while cri-o is stopped" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	conmon_pid=$(jq '.containerMonitorProcess.pid' "$TESTDIR/crio/overlay-containers/$ctr_id/userdata/state.json")

	stop_crio
	kill -9 "$conmon_pid"
	start_crio
	wait_for_log "Failed to load conmon process for container $ctr_id"
}

@test "cri-o doesn't monitor conmon when the container state doesn't have containerMonitorProcess" {
	start_crio
	ctr_id=$(crictl run "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	conmon_pid=$(jq '.containerMonitorProcess.pid' "$TESTDIR/crio/overlay-containers/$ctr_id/userdata/state.json")

	stop_crio
	jq ".containerMonitorProcess = null" "$TESTDIR/crio/overlay-containers/$ctr_id/userdata/state.json" > "$TESTDIR/$ctr_id-state.json"
	cp "$TESTDIR/$ctr_id-state.json" "$TESTDIR/crio/overlay-containers/$ctr_id/userdata/state.json"
	start_crio
	wait_for_log "Skipping loading conmon process for container $ctr_id"
}
