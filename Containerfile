FROM fedora:latest

RUN dnf update -y && \
        dnf install -y jq \
        vim \
        systemd \
        bats \
        cri-tools \
        containernetworking-plugins \
        conmon \
        containers-common \
        device-mapper-devel \
        git \
        make \
        glib2-devel \
        glibc-devel \
        glibc-static \
        runc \
        libassuan \
        libassuan-devel \
        libgpg-error \
        libseccomp-devel \
        libselinux \
        pkgconf-pkg-config \
        gpgme-devel \
        gcc-go \
        btrfs-progs-devel \
        python3 \
        socat \
        nftables \
        iptables-nft \
        net-tools \
        procps \
        wget \
        bash-completion

WORKDIR /root

RUN mkdir -p /root/go && \
        mkdir -p /opt/cni/bin && \
        wget https://go.dev/dl/go1.21.7.linux-amd64.tar.gz && \
        rm -rf /usr/local/go && tar -C /usr/local -xzf go1.21.7.linux-amd64.tar.gz && \
        echo "export PATH=/usr/local/go/bin:$PATH" >> /root/.bashrc && \
        echo "export GOPATH=/root/go" >> /root/.bashrc && \
        echo "for i in \$(ls /usr/libexec/cni/);do if [ ! -f /opt/cni/bin/\$i ]; then ln -s /usr/libexec/cni/\$i /opt/cni/bin/\$i; fi done" >> /root/.bashrc

CMD ["/sbin/init"]
