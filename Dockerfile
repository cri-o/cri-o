FROM golang:1.7.3

# libseccomp in jessie is not _quite_ new enough -- need backports version
RUN echo 'deb http://httpredir.debian.org/debian jessie-backports main' > /etc/apt/sources.list.d/backports.list

RUN apt-get update && apt-get install -y \
    build-essential \
    curl \
    gawk \
    iptables \
    pkg-config \
    libaio-dev \
    libcap-dev \
    libprotobuf-dev \
    libprotobuf-c0-dev \
    libseccomp2/jessie-backports \
    libseccomp-dev/jessie-backports \
    protobuf-c-compiler \
    protobuf-compiler \
    python-minimal \
    libglib2.0-dev \
    libapparmor-dev \
    --no-install-recommends

# install bats
RUN cd /tmp \
    && git clone https://github.com/sstephenson/bats.git \
    && cd bats \
    && git reset --hard 03608115df2071fff4eaaff1605768c275e5f81f \
    && ./install.sh /usr/local

# install criu
ENV CRIU_VERSION 1.7
RUN mkdir -p /usr/src/criu \
    && curl -sSL https://github.com/xemul/criu/archive/v${CRIU_VERSION}.tar.gz | tar -v -C /usr/src/criu/ -xz --strip-components=1 \
    && cd /usr/src/criu \
    && make install-criu

# Install runc
ENV RUNC_COMMIT cc29e3dded8e27ba8f65738f40d251c885030a28
RUN set -x \
	&& export GOPATH="$(mktemp -d)" \
	&& git clone https://github.com/opencontainers/runc.git "$GOPATH/src/github.com/opencontainers/runc" \
	&& cd "$GOPATH/src/github.com/opencontainers/runc" \
	&& git checkout -q "$RUNC_COMMIT" \
	&& make static BUILDTAGS="seccomp selinux" \
	&& cp runc /usr/local/bin/runc \
	&& rm -rf "$GOPATH"

WORKDIR /go/src/github.com/kubernetes-incubator/cri-o

ADD . /go/src/github.com/kubernetes-incubator/cri-o
