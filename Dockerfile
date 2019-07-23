FROM circleci/golang:1.12
USER root

RUN echo "deb http://apt.llvm.org/stretch/ llvm-toolchain-stretch main" | \
    tee -a /etc/apt/sources.list.d/llvm.list && \
    wget -O - http://apt.llvm.org/llvm-snapshot.gpg.key| apt-key add -

RUN apt-get update &&\
    apt-get install -y \
    apparmor \
    autoconf \
    automake \
    bison \
    bsdmainutils \
    btrfs-tools \
    build-essential \
    clang-format \
    e2fslibs-dev \
    gawk \
    gettext \
    iptables \
    libaio-dev \
    libapparmor-dev \
    libcap-dev \
    libdevmapper-dev \
    libdevmapper1.02.1 \
    libfuse-dev \
    libglib2.0-dev \
    libgpgme11-dev \
    liblzma-dev \
    libnet-dev \
    libnl-3-dev \
    libprotobuf-c-dev \
    libprotobuf-dev \
    libseccomp-dev \
    libseccomp2 \
    libsystemd-dev \
    libtool \
    libudev-dev \
    protobuf-c-compiler \
    protobuf-compiler \
    python-protobuf \
    socat &&\
    apt-get clean

# Install bats
RUN cd /tmp &&\
    git clone https://github.com/bats-core/bats-core.git --depth=1 &&\
    cd bats-core &&\
    ./install.sh /usr &&\
    rm -rf /tmp/bats-core &&\
    mkdir -p ~/.parallel && touch ~/.parallel/will-cite

# Install crictl and critest
ENV CRICTL_COMMIT v1.14.0
RUN wget -qO- https://github.com/kubernetes-sigs/cri-tools/releases/download/$CRICTL_COMMIT/crictl-$CRICTL_COMMIT-linux-amd64.tar.gz \
        | tar xfz - -C /usr/bin &&\
    wget -qO- https://github.com/kubernetes-sigs/cri-tools/releases/download/$CRICTL_COMMIT/critest-$CRICTL_COMMIT-linux-amd64.tar.gz \
        | tar xfz - -C /usr/bin

# Install runc
RUN VERSION=v1.0.0-rc8 &&\
    wget -q -O /usr/bin/runc https://github.com/opencontainers/runc/releases/download/$VERSION/runc.amd64 &&\
    chmod +x /usr/bin/runc

# Install CNI plugins
RUN VERSION=v0.8.1 &&\
    mkdir -p /opt/cni/bin &&\
    wget -qO- https://github.com/containernetworking/plugins/releases/download/$VERSION/cni-plugins-linux-amd64-$VERSION.tgz \
        | tar xfz - -C /opt/cni/bin

# Make sure we have some policy for pulling images
RUN mkdir -p /etc/containers
COPY test/policy.json /etc/containers/policy.json
COPY test/redhat_sigstore.yaml /etc/containers/registries.d/registry.access.redhat.com.yaml
