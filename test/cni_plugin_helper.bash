#!/bin/bash

# This script wraps the CNI 'bridge' plugin to provide additional testing
# capabilities

IFS=';' read -ra array <<< "${CNI_ARGS}"
for arg in "${array[@]}"; do
	IFS='=' read -ra item <<< "${arg}"
	if [[ "${item[0]}" = "K8S_POD_NAMESPACE" ]]; then
		K8S_POD_NAMESPACE="${item[1]}"
	elif [[ "${item[0]}" = "K8S_POD_NAME" ]]; then
		K8S_POD_NAME="${item[1]}"
	fi
done

if [[ -z "${CNI_CONTAINERID}" ]]; then
	exit 1
fi
K8S_POD_NAMESPACE=${K8S_POD_NAMESPACE:-}
K8S_POD_NAME=${K8S_POD_NAME:-}
if [[ "${CNI_COMMAND}" != "VERSION" ]]; then
 if [[ -z "${K8S_POD_NAMESPACE}" ]]; then
	exit 1
 elif [[ -z "${K8S_POD_NAME}" ]]; then
	exit 1
 fi
fi
echo "FOUND_CNI_CONTAINERID=${CNI_CONTAINERID}" >> /tmp/plugin_test_args.out
echo "FOUND_K8S_POD_NAMESPACE=${K8S_POD_NAMESPACE}" >> /tmp/plugin_test_args.out
echo "FOUND_K8S_POD_NAME=${K8S_POD_NAME}" >> /tmp/plugin_test_args.out

if [[ "${CNI_COMMAND}" != "VERSION" ]]; then
  . /tmp/cni_plugin_helper_input.env
  rm -f /tmp/cni_plugin_helper_input.env
fi

result=$(/opt/cni/bin/bridge $@) || exit $?

if [[ "${DEBUG_ARGS}" == "malformed-result" ]]; then
	cat <<-EOF
{
   adsfasdfasdfasfdasdfsadfsafd
}
EOF

else
	echo $result
fi
