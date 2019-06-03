FROM golang:1.12

RUN apt-get update && apt-get install -y \
    apparmor \
    autoconf \
    automake \
    bison \
    build-essential \
    curl \
    e2fslibs-dev \
    gawk \
    gettext \
    iptables \
    pkg-config \
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
    python-minimal \
    python-protobuf \
    libglib2.0-dev \
    btrfs-tools \
    libdevmapper1.02.1 \
    libdevmapper-dev \
    libgpgme11-dev \
    liblzma-dev \
    netcat \
    socat \
    --no-install-recommends \
    bsdmainutils \
    && apt-get clean

# install bats
ENV BATS_COMMIT 8789f910812afbf6b87dd371ee5ae30592f1423f
RUN cd /tmp \
    && git clone https://github.com/bats-core/bats-core.git \
    && cd bats-core \
    && git checkout -q "$BATS_COMMIT" \
    && ./install.sh /usr/local
RUN mkdir -p ~/.parallel && touch ~/.parallel/will-cite

# install criu
ENV CRIU_VERSION 3.9
RUN mkdir -p /usr/src/criu \
    && curl -sSL https://github.com/xemul/criu/archive/v${CRIU_VERSION}.tar.gz | tar -v -C /usr/src/criu/ -xz --strip-components=1 \
    && cd /usr/src/criu \
    && make install-criu \
    && rm -rf /usr/src/criu

# Install runc
RUN VERSION=v1.0.0-rc8 &&\
    wget -q -O /usr/bin/runc https://github.com/opencontainers/runc/releases/download/$VERSION/runc.amd64 &&\
    chmod +x /usr/bin/runc

# Install CNI plugins
RUN VERSION=v0.8.0 &&\
    mkdir -p /opt/cni/bin &&\
    wget -qO- https://github.com/containernetworking/plugins/releases/download/$VERSION/cni-plugins-linux-amd64-$VERSION.tgz \
        | tar xfz - -C /opt/cni/bin

# Install crictl
ENV CRICTL_COMMIT ff8d2e81baf8ff720fb916e42da57c2b772bd19e
RUN set -x \
       && export GOPATH="$(mktemp -d)" \
       && git clone https://github.com/kubernetes-sigs/cri-tools.git "$GOPATH/src/github.com/kubernetes-sigs/cri-tools" \
       && cd "$GOPATH/src/github.com/kubernetes-sigs/cri-tools" \
       && git checkout -q "$CRICTL_COMMIT" \
       && go install github.com/kubernetes-sigs/cri-tools/cmd/crictl \
       && cp "$GOPATH"/bin/crictl /usr/bin/ \
       && rm -rf "$GOPATH"

# Make sure we have some policy for pulling images
RUN mkdir -p /etc/containers
COPY test/policy.json /etc/containers/policy.json
COPY test/redhat_sigstore.yaml /etc/containers/registries.d/registry.access.redhat.com.yaml

WORKDIR /go/src/github.com/cri-o/cri-o

ADD . /go/src/github.com/cri-o/cri-o
