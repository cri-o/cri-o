#!/bin/bash

if [[ -z "${CNI_ARGS}" ]]; then
	exit 1
fi

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
elif [[ -z "${K8S_POD_NAMESPACE}" ]]; then
	exit 1
elif [[ -z "${K8S_POD_NAME}" ]]; then
	exit 1
fi

echo "FOUND_CNI_CONTAINERID=${CNI_CONTAINERID}" >> /tmp/plugin_test_args.out
echo "FOUND_K8S_POD_NAMESPACE=${K8S_POD_NAMESPACE}" >> /tmp/plugin_test_args.out
echo "FOUND_K8S_POD_NAME=${K8S_POD_NAME}" >> /tmp/plugin_test_args.out

cat <<-EOF
{
  "cniVersion": "0.2.0",
  "ip4": {
    "ip": "1.1.1.1/24"
  }
}
EOF

