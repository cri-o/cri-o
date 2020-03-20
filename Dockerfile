FROM golang:1.11

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
    libcap-dev \
    libfuse-dev \
    libnet-dev \
    libnl-3-dev \
    libostree-dev \
    libprotobuf-dev \
    libprotobuf-c-dev \
    libseccomp2 \
    libseccomp-dev \
    libtool \
    libudev-dev \
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
ENV BATS_COMMIT v1.1.0
RUN cd /tmp \
    && git clone https://github.com/bats-core/bats-core.git \
    && cd bats-core \
    && git checkout -q "$BATS_COMMIT" \
    && ./install.sh /usr/local

# install criu
ENV CRIU_VERSION 3.9
RUN mkdir -p /usr/src/criu \
    && curl -sSL https://github.com/xemul/criu/archive/v${CRIU_VERSION}.tar.gz | tar -v -C /usr/src/criu/ -xz --strip-components=1 \
    && cd /usr/src/criu \
    && make install-criu \
    && rm -rf /usr/src/criu

# Install runc
ENV RUNC_COMMIT 10d38b660a77168360df3522881e2dc2be5056bd
RUN set -x \
	&& export GOPATH="$(mktemp -d)" \
	&& git clone https://github.com/opencontainers/runc.git "$GOPATH/src/github.com/opencontainers/runc" \
	&& cd "$GOPATH/src/github.com/opencontainers/runc" \
	&& git fetch origin --tags \
	&& git checkout -q "$RUNC_COMMIT" \
	&& make static BUILDTAGS="seccomp selinux" \
	&& cp runc /usr/bin/runc \
	&& rm -rf "$GOPATH"

# Install CNI plugins
ENV CNI_COMMIT dcf7368eeab15e2affc6256f0bb1e84dd46a34de
RUN set -x \
       && export GOPATH="$(mktemp -d)" \
       && git clone https://github.com/containernetworking/plugins.git "$GOPATH/src/github.com/containernetworking/plugins" \
       && cd "$GOPATH/src/github.com/containernetworking/plugins" \
       && git checkout -q "$CNI_COMMIT" \
       && ./build.sh \
       && mkdir -p /opt/cni/bin \
       && cp bin/* /opt/cni/bin/ \
       && rm -rf "$GOPATH"

# Install CNI bridge plugin test wrapper
COPY test/cni_plugin_helper.bash /opt/cni/bin/cni_plugin_helper.bash

# Install crictl
ENV CRICTL_COMMIT 98eea54af789ae13edce79cba101fb9ac8e7b241
RUN set -x \
       && export GOPATH="$(mktemp -d)" \
       && git clone https://github.com/kubernetes-sigs/cri-tools.git "$GOPATH/src/github.com/kubernetes-sigs/cri-tools" \
       && cd "$GOPATH/src/github.com/kubernetes-sigs/cri-tools" \
       && git checkout -q "$CRICTL_COMMIT" \
       && go install github.com/kubernetes-sigs/cri-tools/cmd/crictl \
       && cp "$GOPATH"/bin/crictl /usr/bin/ \
       && rm -rf "$GOPATH"

# Install conmon
RUN VERSION=v2.0.0 &&\
    git clone https://github.com/containers/conmon &&\
	cd conmon && git checkout $VERSION &&\
	make && make PREFIX=/ install && cd .. && rm -rf conmon/

# Make sure we have some policy for pulling images
RUN mkdir -p /etc/containers
COPY test/policy.json /etc/containers/policy.json
COPY test/redhat_sigstore.yaml /etc/containers/registries.d/registry.access.redhat.com.yaml

WORKDIR /go/src/github.com/cri-o/cri-o

ADD . /go/src/github.com/cri-o/cri-o
