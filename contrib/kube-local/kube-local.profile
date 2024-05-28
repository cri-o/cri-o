#
# Copyright 2016-2020 The Kubernetes Authors.
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

#####################################################################################
# General Settings                                                                  #
#####################################################################################
# Enable debug messages: true or false
PRINT_DEBUG_ENABLED=false

# The project exists and no need to clone/build again
# Name and value inspired from errno.h
EEXIST=17

# Number of jobs for make command
MAKE_NUMBER_OF_JOBS="-j 4"

# Config section begin
CONFIG_HEADER="# kube-local section begins"

# Config section end
CONFIG_HEADER_END="# kube-local section ends"

# A kubectl script or command to be executed
# Examples:
#   Example 1)
#      EXECUTE_SCRIPT="kubectl create ns logging"
#   Example 2)
#      Requires: chmod +x my-super-kubectl-script.sh
#      EXECUTE_SCRIPT="/path/my-super-kubectl-script.sh"
#
KUBECTL_SCRIPT=""

# Output file (please specify full path)
OUTPUT_FILENAME=""

# Run container orchestration service once and exit
RUN_SERVICE_ONCE=false

#####################################################################################
# Auto-install  - Use -y for yes or comment this line for no                        #
#####################################################################################
# Do NOT ask user questions
AUTOINSTALL=false

if [ "${AUTOINSTALL}" = "true" ]; then
    AUTO_INSTALL_QUIET_MODE=false

    # Do NOT ask when RUNTIME and CONTAINER ORCHESTRATION are in diff versions
    FORCE_INSTALL_DIFFER_VERSIONS_RUNTIME_CONTAINER_ORCHESTRATION=true

    # Do NOT ask for update the system | -y = autoyes | -q quiet
    AUTO_UPDATE_SYSTEM="-y -q"

    # Do NOT ask for installing packages | -y = autoyes | -q quiet
    AUTO_INSTALL_PACKAGES="-y -q"

    # Set cgroupv1 (only supported at this time)
    AUTO_SET_CGROUPV1=true

    # Do NOT ask for rebooting the machine
    AUTO_REBOOT=true

    # Remove all files/dirs automatically after usage
    AUTO_CLEAN=true

    # Remove existing golang from distro packaging
    AUTO_REMOVE_EXISTING_GOLANG_FROM_DISTRO_PACKAGING=true

    # Remove existing golang dir and packaging
    AUTO_REMOVE_GOLANG_DIR=true
    AUTO_REMOVE_GOLANG_PACKAGE_IF_EXISTS="-y -q"

    # auto install the optional package(s) for runtime? - true or false
    AUTOINSTALL_RUNTIME_OPTIONAL_PACKAGES=false
fi


#####################################################################################
# Logging                                                                           #
#####################################################################################
# Enable Logging - true or false
LOGGING=true

# Logging filter
LOGGING_FILTER="DEBUG INFO WARNING FAIL OK"

# Log dir
LOGDIR="/tmp"

# Full log path and name
LOG_FILE="${LOGDIR}/kube-local-$(date +%d-%m-%Y_%H:%M:%S.log)"

#####################################################################################
# DISTRO                                                                            #
#####################################################################################
DISTRO_NAME=""
DISTRO_CMD_TOOL=""
DISTRO_PKG_MANAGER=""
DISTRO_VERSION_ID=""

##############################################################################
# GOLANG                                                                     #
##############################################################################
#                                                                            #
# Why golang from .tar file?                                                 #
# ===========================                                                #
# Some recent Kubernetes require specific golang version which might         #
# not be available from distros repositories.                                #
#                                                                            #
# ARCH Selection                                                             #
# =====================                                                      #
# Supported ARCHs by go (at this moment):                                    #
# 386, arm, arm64, ppc64, mips, mips64, s390x, wasm                          #
#                                                                            #
# Default: amd64 (also known as x86-64) - A mature implementation.           #
##############################################################################
GOLANG_ARCH="linux-amd64"
GOLANG_VERSION="1.13.7"
GOLANG_TAG=""
GOLANG_TAR_FILENAME="go${GOLANG_VERSION}.${GOLANG_ARCH}.tar.gz"
GOLANG_HASH="sha256"
GOLANG_HASH_FILENAME="go${GOLANG_VERSION}.${GOLANG_ARCH}.tar.gz.${GOLANG_HASH}"
GOLANG_TAR_URL="https://dl.google.com/go/${GOLANG_TAR_FILENAME}"
GOLANG_HASH_URL="https://dl.google.com/go/${GOLANG_HASH_FILENAME}"
GOLANG_DIR_INSTALL="/usr/local"
GOLANG_DIR_PATH="${GOLANG_DIR_INSTALL}/go"
GOLANG_DIR_PATH_BIN="${GOLANG_DIR_INSTALL}/go/bin"
GOLANG_PROJECT_HOME="${HOME}/go"
GOLANG_PROJECT_HOME_SRC="${GOLANG_PROJECT_HOME}/src"
GOLANG_PROJECT_HOME_SRC_GITHUB="${GOLANG_PROJECT_HOME_SRC}/github.com"
GOLANG_PROJECT_HOME_BIN="${GOLANG_PROJECT_HOME}/bin"
GOLANG_BIN="${GOLANG_DIR_INSTALL}/go/bin/go"
GO_ENV_VARS="/etc/profile"

##############################################################################
# RUNTIME                                                                    #
##############################################################################
RUNTIME_NAME="cri-o"
RUNTIME_GITHUB_URL="https://github.com/${RUNTIME_NAME}/${RUNTIME_NAME}"

RUNTIME_OPTIONAL_PACKAGES=""

RUNTIME_VERSION="master"
RUNTIME_TAG=""
RUNTIME_GOLANG_SRC_GITHUB="github.com"
RUNTIME_GOLANG_DIR="${GOLANG_PROJECT_HOME_SRC}/${RUNTIME_GOLANG_SRC_GITHUB}/${RUNTIME_NAME}"
RUNTIME_ENDPOINT="/var/run/crio/crio.sock"

##############################################################################
# RUNTIME  CLIENT                                                            #
##############################################################################
RUNTIME_CLIENT_NAME="cri-tools"
RUNTIME_CLIENT_GITHUB_URL="https://github.com/kubernetes-sigs/${RUNTIME_CLIENT_NAME}"
RUNTIME_CLIENT_VERSION="master"
RUNTIME_CLIENT_TAG=""
RUNTIME_CLIENT_GOLANG_SRC_GITHUB="github.com"
RUNTIME_CLIENT_GOLANG_DIR="${GOLANG_PROJECT_HOME_SRC}/${RUNTIME_CLIENT_GOLANG_SRC_GITHUB}/${RUNTIME_CLIENT_NAME}"

##############################################################################
# CONTAINER RUNTIME MONITOR                                                  #
##############################################################################
RUNTIME_MONITOR="conmon"
RUNTIME_MONITOR_GITHUB_URL="https://github.com/containers/${RUNTIME_MONITOR}"
RUNTIME_MONITOR_VERSION="master"
RUNTIME_MONITOR_TAG=""
RUNTIME_MONITOR_GOLANG_SRC_GITHUB="github.com"
RUNTIME_MONITOR_GOLANG_DIR="${GOLANG_PROJECT_HOME_SRC}/${RUNTIME_MONITOR_GOLANG_SRC_GITHUB}/${RUNTIME_MONITOR}"

##############################################################################
# CONTAINER ORCHESTRATION                                                    #
##############################################################################
CONTAINER_ORCHESTRATION_NAME="kubernetes"
CONTAINER_ORCHESTRATION_GITHUB_URL="https://github.com/${CONTAINER_ORCHESTRATION_NAME}/${CONTAINER_ORCHESTRATION_NAME}"
CONTAINER_ORCHESTRATION_VERSION="master"
CONTAINER_ORCHESTRATION_TAG=""
CONTAINER_ORCHESTRATION_GOLANG_SRC_GITHUB="github.com"
CONTAINER_ORCHESTRATION_GOLANG_DIR="${GOLANG_PROJECT_HOME_SRC}/${CONTAINER_ORCHESTRATION_GOLANG_SRC_GITHUB}/${CONTAINER_ORCHESTRATION_NAME}"
CONTAINER_ORCHESTRATION_KUBECONFIG_NAME="admin.kubeconfig"
CONTAINER_ORCHESTRATION_KUBECONFIG_PATH="/var/run/${CONTAINER_ORCHESTRATION_NAME}/${CONTAINER_ORCHESTRATION_KUBECONFIG_NAME}"
CONTAINER_ORCHESTRATION_OUTPUT_DIR="${CONTAINER_ORCHESTRATION_GOLANG_DIR}/_output/bin"
CONTAINER_ORCHESTRATION_ETCD="${CONTAINER_ORCHESTRATION_GOLANG_DIR}/third_party/etcd"
CONTAINER_ORCHESTRATION_SERVICES="kube-controller-manager kube-apiserver kubelet kube-scheduler etcd conmon kube-proxy local-up-cluster.sh dnsmasq /pause /sidecar"
CONTAINER_ORCHESTRATION_LOGS="etcd.log kube-apiserver.log kube-proxy.log kube-audit-policy-file kube-proxy.yaml kube-controller-manager.log kube-scheduler.log kubelet.log kube-serviceaccount.key"
CONTAINER_ORCHESTRATION_LOG_KUBE_APISERVER_AUDIT="${LOGDIR}/kube-apiserver-audit.log"
KUBECTL_CMD="${GOLANG_PROJECT_HOME_SRC}/${CONTAINER_ORCHESTRATION_GOLANG_SRC_GITHUB}/${CONTAINER_ORCHESTRATION_NAME}/cluster/kubectl.sh"
KUBECTL_ATTEMPTS="5"
KUBECTL_WAIT_SEC_FOR_NEXT_CMD="60s"

##############################################################################
# CONTAINER NETWORK                                                          #
##############################################################################
CONTAINER_NETWORK_REPO_NAME="containernetworking"
CONTAINER_NETWORK_REPO_PLUGIN_DIR="plugins"
CONTAINER_NETWORK_GOLANG_SRC_GITHUB="github.com"
CONTAINER_NETWORK_GOLANG_DIR="${GOLANG_PROJECT_HOME_SRC}/${CONTAINER_NETWORK_GOLANG_SRC_GITHUB}/${CONTAINER_NETWORK_REPO_NAME}"
CONTAINER_NETWORK_GITHUB_URL="https://github.com/${CONTAINER_NETWORK_REPO_NAME}/${CONTAINER_NETWORK_REPO_PLUGIN_DIR}"
CONTAINER_NETWORK_VERSION=""
CONTAINER_NETWORK_TAG="v0.8.1"
CONTAINER_NETWORK_CNI_PATH="/opt/cni"
CONTAINER_NETWORK_CNI_PATH_BIN="${CONTAINER_NETWORK_CNI_PATH}/bin"
CONTAINER_NETWORK_PLUGINS="bandwidth bridge firewall flannel host-device ipvlan loopback macvlan portmap ptp sbr tuning vlan"

##############################################################################
# Binaries                                                                   #
##############################################################################
WGET_BIN="/usr/bin/wget"
TAR_BIN="/usr/bin/tar"
SYSTEMCTL_BIN="/usr/bin/systemctl"
GIT_BIN="/usr/bin/git"
MAKE_BIN="/usr/bin/make"
MKDIR_BIN="/usr/bin/mkdir"
MV_BIN="/usr/bin/mv"
RM_BIN="/usr/bin/rm"
RPM_BIN="/usr/bin/rpm"
DPKG_BIN="/usr/bin/dpkg"
APT_GET_BIN="/usr/bin/apt-get"
DNF_BIN="/usr/bin/dnf"
ZYPPER_BIN="/usr/bin/zypper"
SED_BIN="/usr/bin/sed"
GRUBBY_BIN="/usr/sbin/grubby"
GREP_BIN="/usr/bin/grep"
AWK_BIN="/usr/bin/awk"
SUDO_BIN="/usr/bin/sudo"
REBOOT_BIN="/usr/sbin/reboot"
SHA256SUM_BIN="/usr/bin/sha256sum"
CAT_BIN="/usr/bin/cat"
HOSTNAME_BIN="/usr/bin/hostname"
PGREP_BIN="/usr/bin/pgrep"
CP_BIN="/usr/bin/cp"
SUBSCRIPTION_MANAGER_BIN="/usr/bin/subscription-manager"
XARGS_BIN="/usr/bin/xargs"
KILL_BIN="/usr/bin/kill"
SLEEP_BIN="/usr/bin/sleep"

##############################################################################
# ADDITIONAL INFO FOR LOCAL CLUSTER                                          #
##############################################################################
IP_LOCALHOST=$(${HOSTNAME_BIN} -I | cut -d ' ' -f1)
FEATURE_GATES="AllAlpha=false,RunAsGroup=true"
PATH_PROJECTS=\${PATH}:${CONTAINER_ORCHESTRATION_OUTPUT_DIR}:${CONTAINER_ORCHESTRATION_ETCD}:${GOLANG_DIR_PATH_BIN}
CONTAINER_RUNTIME=remote
CGROUP_DRIVER=systemd
CONTAINER_RUNTIME_ENDPOINT="${RUNTIME_ENDPOINT}"
ALLOW_SECURITY_CONTEXT=","
ALLOW_PRIVILEGED=1
DNS_SERVER_IP=${IP_LOCALHOST}
API_HOST=${IP_LOCALHOST}
API_HOST_IP=${IP_LOCALHOST}
KUBE_ENABLE_CLUSTER_DNS=true
ENABLE_HOSTPATH_PROVISIONER=true
KUBE_ENABLE_CLUSTER_DASHBOARD=true
KUBECONFIG="${CONTAINER_ORCHESTRATION_KUBECONFIG_PATH}"
KUBERNETES_PROVIDER=local

##############################################################################
# Specific info for CRI-O                                                    #
##############################################################################
MD2MAN_GITHUB_BASE="cpuguy83"
MD2MAN_NAME="go-md2man"
MD2MAN_VERSION="master"
MD2MAN_TAG=""
MD2MAN_GOLANG_SRC_GITHUB="github.com"
MD2MAN_GITHUB_URL="${MD2MAN_GOLANG_SRC_GITHUB}/${MD2MAN_GITHUB_BASE}/${MD2MAN_NAME}"
MD2MAN_GOLANG_DIR="${GOLANG_PROJECT_HOME_SRC}/${MD2MAN_GOLANG_SRC_GITHUB}/${MD2MAN_GITHUB_BASE}"
#
########################################
# Fedora/RHEL/CentOS required packages #
########################################
#
# Fedora < 31 | RHEL < 7 | CentOS < 8
CRIO_PACKAGES_FEDORA_RHEL_CENTOS="containers-common \
git \
glib2-devel \
glibc-devel \
glibc-static \
gpgme-devel \
libassuan-devel \
libgpg-error-devel \
libseccomp-devel \
libselinux-devel \
pkgconfig \
gcc \
make \
runc"

# Fedora >= 31 / RHEL >= 8 / CentOS >=8
CRIO_PACKAGES_FEDORA_RHEL_CENTOS_LATEST="containers-common \
git \
glib2-devel \
glibc-devel \
glibc-static \
gpgme-devel \
libassuan-devel \
libgpg-error-devel \
libseccomp-devel \
libselinux-devel \
pkgconf-pkg-config \
make \
gcc \
runc"

########################################
# Ubuntu required packages             #
########################################
CRIO_PACKAGES_UBUNTU="containers-common \
git \
golang-go \
libassuan-dev \
libglib2.0-dev \
libc6-dev \
libgpgme11-dev \
libgpg-error-dev \
libseccomp-dev \
libsystemd-dev \
libselinux1-dev \
pkg-config \
go-md2man \
cri-o-runc \
libudev-dev \
software-properties-common \
gcc \
make"
#
########################################
# OpenSuse required packages           #
########################################
CRIO_PACKAGES_OPENSUSE="FIX-ME"
#
########################################
# Debian required packages             #
########################################
CRIO_PACKAGES_DEBIAN="FIX-ME"
