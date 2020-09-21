#!/usr/bin/env bats

load helpers

function setup() {
  setup_test
  # Start it with the environment variable set, don't use replace_config
  CONTAINER_ENABLE_CUSTOM_SHM_SIZE=true start_crio
}

function teardown() {
  cleanup_test
}

@test "check /dev/shm is changed" {
  # Run base container to ensure it creates at all
  pod_id=$(crictl runp <(cat "$TESTDATA"/sandbox_config.json | jq '.annotations."io.kubernetes.cri-o.ShmSize" = "16Mi"'))

  # Run multi-container pod to ensure that they all work together
  ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

  # Start the container by its ID
  run crictl start "$ctr_id"
  [ "$status" -eq 0 ]

  # Confirm that the new size is applied
  df=$(crictl exec --sync "$ctr_id" df | grep /dev/shm)
  [[ "$df" == *'16384'* ]]
}

@test "check /dev/shm fails with incorrect values" {
  # Ensure pod fails if /dev/shm size is negative
  run crictl runp <(cat "$TESTDATA"/sandbox_config.json | jq '.annotations."io.kubernetes.cri-o.ShmSize" = "-1"')
  [ "$status" -ne 0 ]

  # Ensure pod fails if /dev/shm size is not a size
  run crictl runp <(cat "$TESTDATA"/sandbox_config.json | jq '.annotations."io.kubernetes.cri-o.ShmSize" = "notanumber"')
  [ "$status" -ne 0 ]
}
