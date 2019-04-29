FROM golang:1.12

RUN apt-get update && apt-get install -y \
    apparmor \
    autoconf \
    automake \
    bison \
    build-essential \
    e2fslibs-dev \
    gawk \
    gettext \
    iptables \
    libaio-dev \
    libapparmor-dev \
    libcap-dev \
    libfuse-dev \
    libnet-dev \
    libnl-3-dev \
    libprotobuf-dev \
    libprotobuf-c0-dev \
    libseccomp2 \
    libseccomp-dev \
    libtool \
    libudev-dev \
    libsystemd-dev \
    parallel \
    protobuf-c-compiler \
    protobuf-compiler \
    python-protobuf \
    libglib2.0-dev \
    btrfs-tools \
    libdevmapper1.02.1 \
    libdevmapper-dev \
    libgpgme11-dev \
    liblzma-dev \
    netcat \
    socat \
    bsdmainutils \
    && apt-get clean

# Install bats
RUN cd /tmp &&\
    git clone https://github.com/bats-core/bats-core.git --depth=1 &&\
    cd bats-core &&\
    ./install.sh /usr &&\
    rm -rf /tmp/bats-core &&\
    mkdir -p ~/.parallel && touch ~/.parallel/will-cite

# Install crictl
RUN VERSION=v1.14.0 &&\
    wget -qO- https://github.com/kubernetes-sigs/cri-tools/releases/download/$VERSION/crictl-$VERSION-linux-amd64.tar.gz \
        | tar xfz - -C /usr/bin

# Install runc
RUN VERSION=v1.0.0-rc8 &&\
    wget -q -O /usr/bin/runc https://github.com/opencontainers/runc/releases/download/$VERSION/runc.amd64 &&\
    chmod +x /usr/bin/runc

# Install CNI plugins
RUN VERSION=v0.7.5 &&\
    mkdir -p /opt/cni/bin &&\
    wget -qO- https://github.com/containernetworking/plugins/releases/download/$VERSION/cni-plugins-amd64-$VERSION.tgz \
        | tar xfz - -C /opt/cni/bin

# Make sure we have some policy for pulling images
RUN mkdir -p /etc/containers
COPY test/policy.json /etc/containers/policy.json
COPY test/redhat_sigstore.yaml /etc/containers/registries.d/registry.access.redhat.com.yaml

WORKDIR /go/src/github.com/cri-o/cri-o

ADD . /go/src/github.com/cri-o/cri-o
