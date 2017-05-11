#!/bin/bash

# Copyright 2016 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Due to the GCE custom metadata size limit, we split the entire script into two
# files configure.sh and configure-helper.sh. The functionality of downloading
# kubernetes configuration, manifests, docker images, and binary files are
# put in configure.sh, which is uploaded via GCE custom metadata.

set -o errexit
set -o nounset
set -o pipefail

function set-broken-motd {
  cat > /etc/motd <<EOF
Broken (or in progress) Kubernetes node setup! Check the cluster initialization status
using the following commands.

Master instance:
  - sudo systemctl status kube-master-installation
  - sudo systemctl status kube-master-configuration

Node instance:
  - sudo systemctl status kube-node-installation
  - sudo systemctl status kube-node-configuration
EOF
}

function download-kube-env {
  # Fetch kube-env from GCE metadata server.
  local -r tmp_kube_env="/tmp/kube-env.yaml"
  curl --fail --retry 5 --retry-delay 3 --silent --show-error \
    -H "X-Google-Metadata-Request: True" \
    -o "${tmp_kube_env}" \
    http://metadata.google.internal/computeMetadata/v1/instance/attributes/kube-env
  # Convert the yaml format file into a shell-style file.
  eval $(python -c '''
import pipes,sys,yaml
for k,v in yaml.load(sys.stdin).iteritems():
  print("readonly {var}={value}".format(var = k, value = pipes.quote(str(v))))
''' < "${tmp_kube_env}" > "${KUBE_HOME}/kube-env")
  rm -f "${tmp_kube_env}"
}

function download-kube-master-certs {
  # Fetch kube-env from GCE metadata server.
  local -r tmp_kube_master_certs="/tmp/kube-master-certs.yaml"
  curl --fail --retry 5 --retry-delay 3 --silent --show-error \
    -H "X-Google-Metadata-Request: True" \
    -o "${tmp_kube_master_certs}" \
    http://metadata.google.internal/computeMetadata/v1/instance/attributes/kube-master-certs
  # Convert the yaml format file into a shell-style file.
  eval $(python -c '''
import pipes,sys,yaml
for k,v in yaml.load(sys.stdin).iteritems():
  print("readonly {var}={value}".format(var = k, value = pipes.quote(str(v))))
''' < "${tmp_kube_master_certs}" > "${KUBE_HOME}/kube-master-certs")
  rm -f "${tmp_kube_master_certs}"
}

function validate-hash {
  local -r file="$1"
  local -r expected="$2"

  actual=$(sha1sum ${file} | awk '{ print $1 }') || true
  if [[ "${actual}" != "${expected}" ]]; then
    echo "== ${file} corrupted, sha1 ${actual} doesn't match expected ${expected} =="
    return 1
  fi
}

# Retry a download until we get it. Takes a hash and a set of URLs.
#
# $1 is the sha1 of the URL. Can be "" if the sha1 is unknown.
# $2+ are the URLs to download.
function download-or-bust {
  local -r hash="$1"
  shift 1

  local -r urls=( $* )
  while true; do
    for url in "${urls[@]}"; do
      local file="${url##*/}"
      rm -f "${file}"
      if ! curl -f --ipv4 -Lo "${file}" --connect-timeout 20 --max-time 300 --retry 6 --retry-delay 10 "${url}"; then
        echo "== Failed to download ${url}. Retrying. =="
      elif [[ -n "${hash}" ]] && ! validate-hash "${file}" "${hash}"; then
        echo "== Hash validation of ${url} failed. Retrying. =="
      else
        if [[ -n "${hash}" ]]; then
          echo "== Downloaded ${url} (SHA1 = ${hash}) =="
        else
          echo "== Downloaded ${url} =="
        fi
        return
      fi
    done
  done
}

function split-commas {
  echo $1 | tr "," "\n"
}

function install-gci-mounter-tools {
  CONTAINERIZED_MOUNTER_HOME="${KUBE_HOME}/containerized_mounter"
  mkdir -p "${CONTAINERIZED_MOUNTER_HOME}"
  chmod a+x "${CONTAINERIZED_MOUNTER_HOME}"
  mkdir -p "${CONTAINERIZED_MOUNTER_HOME}/rootfs"
  local -r mounter_tar_sha="8003b798cf33c7f91320cd6ee5cec4fa22244571"
  download-or-bust "${mounter_tar_sha}" "https://storage.googleapis.com/kubernetes-release/gci-mounter/mounter.tar"
  cp "${dst_dir}/kubernetes/gci-trusty/gci-mounter" "${CONTAINERIZED_MOUNTER_HOME}/mounter"
  chmod a+x "${CONTAINERIZED_MOUNTER_HOME}/mounter"
  mv "${KUBE_HOME}/mounter.tar" /tmp/mounter.tar
  tar xvf /tmp/mounter.tar -C "${CONTAINERIZED_MOUNTER_HOME}/rootfs"
  rm /tmp/mounter.tar
  mkdir -p "${CONTAINERIZED_MOUNTER_HOME}/rootfs/var/lib/kubelet"
}

# Install node problem detector binary.
function install-node-problem-detector {
  local -r npd_version="v0.3.0"
  local -r npd_sha1="2e6423c5798e14464271d9c944e56a637ee5a4bc"
  local -r npd_release_path="https://storage.googleapis.com/kubernetes-release"
  local -r npd_tar="node-problem-detector-${npd_version}.tar.gz"
  download-or-bust "${npd_sha1}" "${npd_release_path}/node-problem-detector/${npd_tar}"
  local -r npd_dir="${KUBE_HOME}/node-problem-detector"
  mkdir -p "${npd_dir}"
  tar xzf "${KUBE_HOME}/${npd_tar}" -C "${npd_dir}" --overwrite
  mv "${npd_dir}/bin"/* "${KUBE_HOME}/bin"
  chmod a+x "${KUBE_HOME}/bin/node-problem-detector"
  rmdir "${npd_dir}/bin"
  rm -f "${KUBE_HOME}/${npd_tar}"
}

# Downloads kubernetes binaries and kube-system manifest tarball, unpacks them,
# and places them into suitable directories. Files are placed in /home/kubernetes.
function install-kube-binary-config {
  cd "${KUBE_HOME}"
  local -r server_binary_tar_urls=( $(split-commas "${SERVER_BINARY_TAR_URL}") )
  local -r server_binary_tar="${server_binary_tar_urls[0]##*/}"
  if [[ -n "${SERVER_BINARY_TAR_HASH:-}" ]]; then
    local -r server_binary_tar_hash="${SERVER_BINARY_TAR_HASH}"
  else
    echo "Downloading binary release sha1 (not found in env)"
    download-or-bust "" "${server_binary_tar_urls[@]/.tar.gz/.tar.gz.sha1}"
    local -r server_binary_tar_hash=$(cat "${server_binary_tar}.sha1")
  fi
  echo "Downloading binary release tar"
  download-or-bust "${server_binary_tar_hash}" "${server_binary_tar_urls[@]}"
  tar xzf "${KUBE_HOME}/${server_binary_tar}" -C "${KUBE_HOME}" --overwrite
  # Copy docker_tag and image files to ${KUBE_HOME}/kube-docker-files.
  src_dir="${KUBE_HOME}/kubernetes/server/bin"
  dst_dir="${KUBE_HOME}/kube-docker-files"
  mkdir -p "${dst_dir}"
  cp "${src_dir}/"*.docker_tag "${dst_dir}"
  if [[ "${KUBERNETES_MASTER:-}" == "false" ]]; then
    cp "${src_dir}/kube-proxy.tar" "${dst_dir}"
    if [[ "${ENABLE_NODE_PROBLEM_DETECTOR:-}" == "standalone" ]]; then
      install-node-problem-detector
    fi
  else
    cp "${src_dir}/kube-apiserver.tar" "${dst_dir}"
    cp "${src_dir}/kube-controller-manager.tar" "${dst_dir}"
    cp "${src_dir}/kube-scheduler.tar" "${dst_dir}"
    cp -r "${KUBE_HOME}/kubernetes/addons" "${dst_dir}"
  fi
  local -r kube_bin="${KUBE_HOME}/bin"
  mv "${src_dir}/kubelet" "${kube_bin}"
  mv "${src_dir}/kubectl" "${kube_bin}"

  if [[ "${NETWORK_PROVIDER:-}" == "kubenet" ]] || \
     [[ "${NETWORK_PROVIDER:-}" == "cni" ]]; then
    #TODO(andyzheng0831): We should make the cni version number as a k8s env variable.
    local -r cni_tar="cni-0799f5732f2a11b329d9e3d51b9c8f2e3759f2ff.tar.gz"
    local -r cni_sha1="1d9788b0f5420e1a219aad2cb8681823fc515e7c"
    download-or-bust "${cni_sha1}" "https://storage.googleapis.com/kubernetes-release/network-plugins/${cni_tar}"
    local -r cni_dir="${KUBE_HOME}/cni"
    mkdir -p "${cni_dir}"
    tar xzf "${KUBE_HOME}/${cni_tar}" -C "${cni_dir}" --overwrite
    mv "${cni_dir}/bin"/* "${kube_bin}"
    rmdir "${cni_dir}/bin"
    rm -f "${KUBE_HOME}/${cni_tar}"
  fi

  mv "${KUBE_HOME}/kubernetes/LICENSES" "${KUBE_HOME}"
  mv "${KUBE_HOME}/kubernetes/kubernetes-src.tar.gz" "${KUBE_HOME}"

  # Put kube-system pods manifests in ${KUBE_HOME}/kube-manifests/.
  dst_dir="${KUBE_HOME}/kube-manifests"
  mkdir -p "${dst_dir}"
  local -r manifests_tar_urls=( $(split-commas "${KUBE_MANIFESTS_TAR_URL}") )
  local -r manifests_tar="${manifests_tar_urls[0]##*/}"
  if [ -n "${KUBE_MANIFESTS_TAR_HASH:-}" ]; then
    local -r manifests_tar_hash="${KUBE_MANIFESTS_TAR_HASH}"
  else
    echo "Downloading k8s manifests sha1 (not found in env)"
    download-or-bust "" "${manifests_tar_urls[@]/.tar.gz/.tar.gz.sha1}"
    local -r manifests_tar_hash=$(cat "${manifests_tar}.sha1")
  fi
  echo "Downloading k8s manifests tar"
  download-or-bust "${manifests_tar_hash}" "${manifests_tar_urls[@]}"
  tar xzf "${KUBE_HOME}/${manifests_tar}" -C "${dst_dir}" --overwrite
  local -r kube_addon_registry="${KUBE_ADDON_REGISTRY:-gcr.io/google_containers}"
  if [[ "${kube_addon_registry}" != "gcr.io/google_containers" ]]; then
    find "${dst_dir}" -name \*.yaml -or -name \*.yaml.in | \
      xargs sed -ri "s@(image:\s.*)gcr.io/google_containers@\1${kube_addon_registry}@"
    find "${dst_dir}" -name \*.manifest -or -name \*.json | \
      xargs sed -ri "s@(image\":\s+\")gcr.io/google_containers@\1${kube_addon_registry}@"
  fi
  cp "${dst_dir}/kubernetes/gci-trusty/gci-configure-helper.sh" "${KUBE_HOME}/bin/configure-helper.sh"
  cp "${dst_dir}/kubernetes/gci-trusty/health-monitor.sh" "${KUBE_HOME}/bin/health-monitor.sh"
  chmod -R 755 "${kube_bin}"

  # Install gci mounter related artifacts to allow mounting storage volumes in GCI
  install-gci-mounter-tools
  
  # Clean up.
  rm -rf "${KUBE_HOME}/kubernetes"
  rm -f "${KUBE_HOME}/${server_binary_tar}"
  rm -f "${KUBE_HOME}/${server_binary_tar}.sha1"
  rm -f "${KUBE_HOME}/${manifests_tar}"
  rm -f "${KUBE_HOME}/${manifests_tar}.sha1"
}

######### Main Function ##########
echo "Start to install kubernetes files"
set-broken-motd
KUBE_HOME="/home/kubernetes"
download-kube-env
source "${KUBE_HOME}/kube-env"
if [[ "${KUBERNETES_MASTER:-}" == "true" ]]; then
  download-kube-master-certs
fi
install-kube-binary-config
echo "Done for installing kubernetes files"

