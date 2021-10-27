FROM fedora:29 AS build

COPY --from=registry.ci.openshift.org/rhcos/machine-os-content:4.9 /srv/ /srv/
RUN set -x && yum install -y ostree yum-utils selinux-policy-targeted && \
    commit=$( find /srv -name *.commit | sed -Ee 's|.*objects/(.+)/(.+)\.commit|\1\2|' | head -1 ) && \
    mkdir /tmp/working && cd /tmp/working && \
    yumdownloader -y --disablerepo=* --enablerepo=built --destdir=/tmp/rpms cri-o && \
    ls /tmp/rpms/ && (cd /tmp/rpms/ && ls cri-o*) && \
    for i in $(find /tmp/rpms/ -name cri-o* -iname *.rpm); do echo "Extracting $i ..."; rpm2cpio $i | cpio -div; done && \
    if [[ -d etc ]]; then mv etc usr/; fi && \
    mkdir -p /tmp/tmprootfs/etc && \
    ostree --repo=/srv/repo checkout -U $commit --subpath /usr/etc/selinux /tmp/tmprootfs/etc/selinux && \
    ostree --repo=/srv/repo commit --parent=$commit --tree=ref=$commit --tree=dir=. \
        --selinux-policy /tmp/tmprootfs \
        -s "cri-o-ci-dev overlay RPMs" --branch=cri-o-ci-dev

FROM scratch
COPY --from=build /srv/ /srv/
LABEL io.openshift.build.version-display-names="machine-os=rhcos image for testing CRI-O only- if you see this outside of PR runs for CRI-O- you found an urgent blocker bug" \
      io.openshift.build.versions="machine-os=1.2.3-testing-if-you-see-this-outside-of-PR-runs-for-cri-o-cri-o-you-found-an-urgent-blocker-bug"
