FROM centos

RUN yum-config-manager --nogpgcheck --add-repo https://cbs.centos.org/repos/virt7-container-common-candidate/x86_64/os/ && \
    yum install --disablerepo=extras --nogpgcheck --setopt=tsflags=nodocs -y \
    btrfs-progs-devel \
    device-mapper-devel \
    git \
    glib2-devel \
    glibc-devel \
    glibc-static \
    go \
    golang-github-cpuguy83-go-md2man \
    gpgme-devel \
    libassuan-devel \
    libgpg-error-devel \
    libseccomp-devel \
    libselinux-devel \
    ostree-devel \
    pkgconfig \
    make \
    skopeo-containers \
    # start runtime deps
    runc \
    socat \
    iproute \
    iptables && \
    # end runtime deps
    yum clean all

ENV CRIO_PR ""
ENV CRIO_COMMIT release-1.7
RUN set -x \
       && export GOPATH="$(mktemp -d)" \
       && git clone https://github.com/kubernetes-incubator/cri-o.git "$GOPATH/src/github.com/kubernetes-incubator/cri-o" \
       && cd "$GOPATH/src/github.com/kubernetes-incubator/cri-o" \
       && if [[ -n "$CRIO_PR" ]]; then git fetch origin "pull/$CRIO_PR/head:pr$CRIO_PR" && git checkout "pr$CRIO_PR"; else git checkout -q "$CRIO_COMMIT"; fi \
       && make binaries docs \
       && ./bin/crio --selinux=true --storage-driver=overlay --conmon /usr/libexec/crio/conmon --cni-plugin-dir /usr/libexec/cni --default-mounts /usr/share/rhel/secrets:/run/secrets --cgroup-manager=systemd config > crio.conf \
       && make install.config install.systemd PREFIX=/usr \
       && make install PREFIX=/usr \
       && rm -rf "$GOPATH"

RUN mkdir -p /exports/hostfs/etc/crio /exports/hostfs/opt/cni/bin/ /exports/hostfs/var/lib/containers/storage/ && \
    cp /etc/crio/* /exports/hostfs/etc/crio && \
    if test -e /usr/libexec/cni; then cp -Lr /usr/libexec/cni/* /exports/hostfs/opt/cni/bin/; fi

RUN sed -i '/storage_option =/s/.*/&\n"overlay.override_kernel_check=1",/' /exports/hostfs/etc/crio/crio.conf

COPY manifest.json tmpfiles.template config.json.template service.template /exports/

COPY set_mounts.sh /
COPY run.sh /usr/bin/

CMD ["/usr/bin/run.sh"]
