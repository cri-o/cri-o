#!/usr/bin/env bats

load helpers

function setup() {
  setup_test
  CNI_PLUGIN_NAME="log_cni_plugin"
}

function teardown() {
  cleanup_test
}

function run_pod() {
  crictl runp "$TESTDATA"/sandbox_config.json
}

function delete_pod() {
  id=$(crictl runp "$TESTDATA"/sandbox_config.json)
  crictl stopp "$id"
  crictl rmp "$id"
}

function prepare_cni_plugin() {
  # name the config with prefix 001 to ensure the corresponding cni plugin will be invoked when pod is created
  cat >"$CRIO_CNI_CONFIG"/001-"$CNI_PLUGIN_NAME".conf <<-EOF
{
  "cniVersion": "0.3.1",
  "name": "$CNI_PLUGIN_NAME",
  "type": "$CNI_PLUGIN_NAME.sh",
  "config": {
    "log_path": "$1"
  },
  "capabilities": {
    "io.kubernetes.cri.pod-annotations": $2
  }
}
EOF
  chmod 755 "$TESTDATA"/"$CNI_PLUGIN_NAME".sh
  # copy the cni plugin into cni plugin binary directory
  cp "$TESTDATA"/"$CNI_PLUGIN_NAME".sh "$CRIO_CNI_PLUGIN"/"$CNI_PLUGIN_NAME".sh
}

function prepare_chained_cni_plugins() {
  # create a chained cni plugin configuration file
  cat >"$CRIO_CNI_CONFIG"/001-"$CNI_PLUGIN_NAME".conflist <<-EOF
{
  "cniVersion": "0.3.1",
  "name": "$CNI_PLUGIN_NAME",
  "plugins": [
    {
      "type": "$CNI_PLUGIN_NAME.sh",
      "config": {
        "log_path": "$1"
      },
      "capabilities": {
        "io.kubernetes.cri.pod-annotations": $2
      }
    },
    {
      "type": "$CNI_PLUGIN_NAME.sh",
      "config": {
        "log_path": "$3"
      },
      "capabilities": {
        "io.kubernetes.cri.pod-annotations": $4
      }
    }
  ]
}
EOF
  chmod 755 "$TESTDATA"/"$CNI_PLUGIN_NAME".sh
  cp "$TESTDATA"/"$CNI_PLUGIN_NAME".sh "$CRIO_CNI_PLUGIN"/"$CNI_PLUGIN_NAME".sh
}

function contains() {
  # this function checks whether b contains a
  a=$1
  b=$2
  # if a and b are both null or empty, we consider them equal
  if { [[ $a == null ]] || [[ $a == '{}' ]]; } && { [[ $b == null ]] || [[ $b == '{}' ]]; }; then
    return 0
  fi
  if [[ $a == null ]] || [[ $a == '{}' ]] || [[ $b == null ]] || [[ $b == '{}' ]]; then
    return 1
  fi
  for key in $(echo "$a" | jq 'keys[]'); do
    value=$(jq -e ."$key" <<<"$b")
    # value is null means b does not have this key
    if [[ $value == null ]]; then
      return 1
    # if b has this key, checks their value
    elif [[ $value != $(jq -e ."$key" <<<"$a") ]]; then
      return 1
    fi
  done
  return 0
}

function annotations_equal() {
  cni_plugin_received=$1
  expected=$2
  contains "$cni_plugin_received" "$expected"
  expected_contains_received=$?
  contains "$expected" "$cni_plugin_received"
  received_contains_expected=$?
  [[ $expected_contains_received -eq 0 ]] && [[ $received_contains_expected -eq 0 ]]
}

@test "single cni plugin with pod annotations capability enabled" {
  enable_capability=true
  cni_plugin_log_path="$TESTDIR/cni.log"

  start_crio
  prepare_cni_plugin "$cni_plugin_log_path" $enable_capability
  run_pod
  delete_pod

  annotations_cni_plugin_received=$(jq '.runtimeConfig."io.kubernetes.cri.pod-annotations"' "$cni_plugin_log_path")
  expected=$(jq '.annotations' "$TESTDATA/sandbox_config.json")

  annotations_equal "$annotations_cni_plugin_received" "$expected"
}

@test "single cni plugin with pod annotations capability disabled" {
  enable_capability=false
  cni_plugin_log_path="$TESTDIR/cni.log"

  start_crio
  prepare_cni_plugin "$cni_plugin_log_path" $enable_capability
  run_pod
  delete_pod

  annotations_cni_plugin_received=$(jq '.runtimeConfig."io.kubernetes.cri.pod-annotations"' "$cni_plugin_log_path")
  expected=null

  annotations_equal "$annotations_cni_plugin_received" "$expected"
}

@test "pod annotations capability for chained cni plugins" {
  enable_capability_first=false
  cni_plugin_log_path_first="$TESTDIR/cni-01.log"
  enable_capability_second=true
  cni_plugin_log_path_second="$TESTDIR/cni-02.log"

  start_crio
  prepare_chained_cni_plugins "${cni_plugin_log_path_first}" $enable_capability_first "$cni_plugin_log_path_second" $enable_capability_second
  run_pod
  delete_pod

  annotations_first_cni_plugin_received=$(jq '.runtimeConfig."io.kubernetes.cri.pod-annotations"' "$cni_plugin_log_path_first")
  if [[ $enable_capability_first == true ]]; then
    expected=$(jq '.annotations' "$TESTDATA/sandbox_config.json")
  else
    expected=null
  fi
  annotations_equal "$annotations_first_cni_plugin_received" "$expected"

  annotations_second_cni_plugin_received=$(jq '.runtimeConfig."io.kubernetes.cri.pod-annotations"' "$cni_plugin_log_path_second")
  if [[ $enable_capability_second == true ]]; then
    expected=$(jq '.annotations' "$TESTDATA/sandbox_config.json")
  else
    expected=null
  fi
  annotations_equal "$annotations_second_cni_plugin_received" "$expected"
}
