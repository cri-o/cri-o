#!/usr/bin/env bash
set -euo pipefail

usage() {
    printf "Usage: %s -d CRIO_DIR -i IGNITION_OUT_DIR -b GCS_BUCKET_NAME [ -s GCS_SA_PATH ]  [ -e EXTRA_CONFIG_PATH ]  [ -h ]\n\n" "$(basename "$0")"
    echo "Possible arguments:"

    printf "  -d\tPath to cri-o source\n"
    printf "  -i\tPath to output directory for generated ignition config\n"
    printf "  -b\tName of the GCS bucket for uploading bundle artifacts\n"
    printf "  -s\tPath to GCP service account file (defaults to '' when existing gcloud credentials are used)\n"
    printf "  -e\tPath to directory for additional cri-o config files\n"
    printf "  -h\tShow this help message\n"
}

CRIO_DIR=
IGNITION_OUT_DIR=
GCS_BUCKET_NAME=
GCS_SA_PATH=
EXTRA_CONFIG_PATH=

required_arg_check() {
    if [[ -z "$1" ]]; then
        echo "Specifying $2 with $3 is required"
        exit 1
    fi
}

parse_args() {
    echo "CRI-O build and generate ignition config script for kubernetes node e2e tests."

    while getopts 'd:i:b:s:e:h' OPTION; do
        case "$OPTION" in
        d)
            CRIO_DIR="$OPTARG"
            echo "Using cri-o source from: $CRIO_DIR"
            ;;
        i)
            IGNITION_OUT_DIR="$OPTARG"
            echo "Generated ignition config file will be output to: $IGNITION_OUT_DIR"
            ;;
        b)
            GCS_BUCKET_NAME="$OPTARG"
            echo "Using GCS bucket: gs://$GCS_BUCKET_NAME"
            ;;
        s)
            GCS_SA_PATH="$OPTARG"
            echo "Using GCP service account from: $GCS_SA_PATH"
            ;;
        e)
            EXTRA_CONFIG_PATH="$OPTARG"
            echo "Using additional cri-o config files from: $EXTRA_CONFIG_PATH"
            ;;
        h)
            usage
            exit 0
            ;;
        ?)
            usage
            exit 1
            ;;
        esac
    done

    required_arg_check "${CRIO_DIR}" "CRIO_DIR" "-d"
    required_arg_check "${IGNITION_OUT_DIR}" "IGNITION_OUT_DIR" "-i"
    required_arg_check "${GCS_BUCKET_NAME}" "GCS_BUCKET_NAME" "-b"
}

parse_args "$@"

# Build and bundle cri-o from source
cd "$CRIO_DIR"

sudo -E make clean

make bin/pinns

sudo -E make build-static

# Get the CPU architecture
LOCAL_ARCH=$(uname -m)
if [[ "$LOCAL_ARCH" == x86_64 ]]; then
    ARCH="amd64"
elif [[ "$LOCAL_ARCH" == aarch64 ]]; then
    ARCH="arm64"
else
    echo "Unsupported local architecture: $LOCAL_ARCH"
    exit 1
fi

sudo cp -r bin/static "bin/static-$ARCH"
sudo chown "$USER" -R "bin/static-$ARCH"

make docs
make crio.conf

make bundle

# Upload cri-o bundle to GCS bucket
export GCS_SA_PATH
export GCS_BUCKET_NAME
"$CRIO_DIR/contrib/custom-node-e2e/upload-artifacts.sh"

# Verify cri-o commit SHAs with built artifact
GIT_BRANCH=$(git branch --show-current)
CRIO_TAR_FILE_PATH=$(find build/bundle/ -maxdepth 1 -regex ".*cri\-o.*\.tar\.gz" -printf "%T@ %p\n" | sort -n | head -1 | cut -d " " -f2)
CRIO_SHA=$(echo "$CRIO_TAR_FILE_PATH" | sed "s/build\/bundle\/cri-o.$ARCH.//" | sed 's/.tar.gz//')

if grep "$CRIO_SHA" <"latest-$GIT_BRANCH.txt"; then
    echo "verified SHA id matches"
fi

# Recursively find all additional config files and get their contents
CONFIG_FILES=$(find "$EXTRA_CONFIG_PATH" -not -type d)
CONF_CONTENT=""
if [[ -n "${CONFIG_FILES}" ]]; then
    FILE_COUNTER=40
    for CONFIG_FILE in $CONFIG_FILES; do
        FILE_CONTENT=$(cat "$CONFIG_FILE")
        CONF_CONTENT="$CONF_CONTENT
cat <<EOF >/etc/crio/crio.conf.d/$FILE_COUNTER.conf
$FILE_CONTENT
EOF
"
        FILE_COUNTER=$((FILE_COUNTER + 1))
    done
fi

# Create a custom node-e2e-installer script
RAND_ID=crio-$USER-$(date +"%Y%m%d")-$(echo $RANDOM | md5sum | head -c 8)
OUTPUT_SHELL_SCRIPT=node_e2e_installer-$RAND_ID.sh
OUTPUT_SHELL_SCRIPT_PATH=/tmp/$OUTPUT_SHELL_SCRIPT
cat <<END >"$OUTPUT_SHELL_SCRIPT_PATH"
#!/usr/bin/env bash
set -euo pipefail

# Commit to run upstream node e2e tests
NODE_E2E_COMMIT=${CRIO_SHA}

install_crio() {
    # Download and install CRIO
    curl --fail --retry 5 --retry-delay 3 --silent --show-error -o /usr/local/crio-install.sh https://raw.githubusercontent.com/cri-o/cri-o/main/scripts/get
    bash /usr/local/crio-install.sh -t "\$NODE_E2E_COMMIT" -b ${GCS_BUCKET_NAME}

    # Setup SELinux labels
    mkdir -p /var/lib/kubelet
    chcon -R -u system_u -r object_r -t var_lib_t /var/lib/kubelet

    mount /tmp /tmp -o remount,exec,suid

    # Remove unwanted cni configuration files
    rm -f /etc/cni/net.d/87-podman-bridge.conflist

    # Setup log level
    echo "CONTAINER_LOG_LEVEL=debug" >>/etc/sysconfig/crio

    cat <<EOF >/etc/crio/crio.conf.d/10-crun.conf
[crio.runtime]
[crio.runtime.runtimes]
[crio.runtime.runtimes.test-handler]
EOF

    cat <<EOF >/etc/crio/crio.conf.d/20-runc.conf
[crio.runtime]
default_runtime = "runc"
[crio.runtime.runtimes]
[crio.runtime.runtimes.runc]
EOF

    cat <<EOF >/etc/crio/crio.conf.d/30-infra-container.conf
[crio.runtime]
drop_infra_ctr = false
EOF

${CONF_CONTENT}
}

install_crio

# Finally start crio
systemctl enable crio.service
systemctl start crio.service
END

# Upload the node-e2e-installer script to GCS bucket
gsutil cp "$OUTPUT_SHELL_SCRIPT_PATH" "gs://$GCS_BUCKET_NAME/$OUTPUT_SHELL_SCRIPT"
GCS_E2E_INSTALLER_SCRIPT_URL=https://storage.googleapis.com/$GCS_BUCKET_NAME/$OUTPUT_SHELL_SCRIPT

# Create a custom ignition config file
export IGN_FILE_PATH=$IGNITION_OUT_DIR/$RAND_ID.ign

# Save the ignition config file to disk
cat <<EOF >"$IGN_FILE_PATH"
{
  "ignition": {
    "version": "3.3.0"
  },
  "kernelArguments": {
    "shouldExist": [
      "systemd.unified_cgroup_hierarchy=0"
    ]
  },
  "storage": {
    "files": [
      {
        "path": "/etc/zincati/config.d/90-disable-auto-updates.toml",
        "contents": {
          "source": "data:,%5Bupdates%5D%0Aenabled%20%3D%20false%0A"
        },
        "mode": 420
      }
    ]
  },
  "systemd": {
    "units": [
      {
        "contents": "[Unit]\nDescription=Download and install dbus-tools.\nBefore=crio-install.service\nAfter=network-online.target\n\n[Service]\nType=oneshot\nExecStart=/usr/bin/rpm-ostree install --apply-live --allow-inactive dbus-tools\n\n[Install]\nWantedBy=multi-user.target\n",
        "enabled": true,
        "name": "dbus-tools-install.service"
      },
      {
        "contents": "[Unit]\nDescription=Download and install crio binaries and configurations.\nAfter=network-online.target\n\n[Service]\nType=oneshot\nExecStartPre=/usr/bin/bash -c '/usr/bin/curl --fail --retry 5 --retry-delay 3 --silent --show-error -o /usr/local/crio-nodee2e-installer.sh  ${GCS_E2E_INSTALLER_SCRIPT_URL}; ln -s /usr/bin/runc /usr/local/bin/runc'\nExecStart=/usr/bin/bash /usr/local/crio-nodee2e-installer.sh\n\n[Install]\nWantedBy=multi-user.target\n",
        "enabled": true,
        "name": "crio-install.service"
      }
    ]
  }
}
EOF
echo "Written custom cri-o ignition file to '$IGN_FILE_PATH'"
