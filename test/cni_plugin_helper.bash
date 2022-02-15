#!/usr/bin/env bash

# This script wraps the CNI 'bridge' plugin to provide additional testing
# capabilities

if [[ "${CNI_COMMAND}" == "VERSION" ]]; then
    echo '{"cniVersion": "0.3.1", "supportedVersions": ["0.4.0","0.3.1"]}'
    exit 0
fi
￼

if [[ -z "${CNI_ARGS}" ]]; then
    exit 1
fi

IFS=';' read -ra array <<<"${CNI_ARGS}"
for arg in "${array[@]}"; do
    IFS='=' read -ra item <<<"${arg}"
    if [[ "${item[0]}" == "K8S_POD_NAMESPACE" ]]; then
        K8S_POD_NAMESPACE="${item[1]}"
    elif [[ "${item[0]}" == "K8S_POD_NAME" ]]; then
        K8S_POD_NAME="${item[1]}"
    elif [[ "${item[0]}" == "K8S_POD_UID" ]]; then
        K8S_POD_UID="${item[1]}"
    fi
done

if [[ -z "${CNI_CONTAINERID}" ]]; then
    exit 1
elif [[ -z "${K8S_POD_NAMESPACE}" ]]; then
    exit 1
elif [[ -z "${K8S_POD_NAME}" ]]; then
    exit 1
elif [[ -z "${K8S_POD_UID}" ]]; then
    exit 1
fi

TEST_DIR=%TEST_DIR%

cat <<EOT >"$TEST_DIR/plugin_test_args.out"
FOUND_CNI_CONTAINERID="${CNI_CONTAINERID}"
FOUND_K8S_POD_NAMESPACE="${K8S_POD_NAMESPACE}"
FOUND_K8S_POD_NAME="${K8S_POD_NAME}"
FOUND_K8S_POD_UID="${K8S_POD_UID}"
EOT

# shellcheck disable=SC1091
. "$TEST_DIR"/cni_plugin_helper_input.env
rm -f "$TEST_DIR"/cni_plugin_helper_input.env

result=$(/opt/cni/bin/bridge "$@") || exit $?

if [[ "${DEBUG_ARGS}" == "malformed-result" ]]; then
    cat <<-EOF
{
   adsfasdfasdfasfdasdfsadfsafd
}
EOF

else
    echo "$result"
fi
