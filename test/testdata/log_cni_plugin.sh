#!/usr/bin/env bash

# this simple cni plugin just log stdin into file

config=$(cat /dev/stdin)

case $CNI_COMMAND in
ADD)
    log=$(jq -r '.config.log_path' <<<"$config")
    if [[ "$log" != null ]]; then
        echo "$config" >>"$log"
    fi
    output='
{
  "cniVersion": "0.3.1"
}'
    echo "$output"
    ;;

DEL) ;;

GET) ;;

VERSION)
    echo '{
  "cniVersion": "0.3.1",
  "supportedVersions": [ "0.1.0", "0.2.0", "0.3.0", "0.3.1", "0.4.0", "1.0.0" ]
}'
    ;;

*)
    echo "Unknown CNI command: $CNI_COMMAND"
    exit 1
    ;;

esac
