#!/usr/bin/env bash

# This script generates release zips and RPMs into _output/releases.
# tito and other build dependencies are required on the host. We will
# be running `hack/build-cross.sh` under the covers, so we transitively
# consume all of the relevant envars.
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

os::util::ensure::system_binary_exists rpmbuild
os::util::ensure::system_binary_exists createrepo

os::build::rpm::get_nvra_vars

OS_RPM_SPECFILE="$(find "${OS_ROOT}" -name *cri-o.spec)"
OS_RPM_NAME="$(rpmspec -q --qf '%{name}\n' "${OS_RPM_SPECFILE}" | head -1)"

os::log::info "Building release RPMs for ${OS_RPM_SPECFILE} ..."

rpm_tmp_dir="${BASETMPDIR}/rpm"
ci_data="${OS_ROOT}/contrib/test/ci"
# RPM requires the spec file be owned by the invoking user
chown "$(id -u):$(id -g)" "${OS_RPM_SPECFILE}" || true

mkdir -p "${rpm_tmp_dir}/SOURCES"
tar czf "${rpm_tmp_dir}/SOURCES/${OS_RPM_NAME}-test.tar.gz" \
    --owner=0 --group=0 \
    --exclude=_output --exclude=.git --transform "s|^|${OS_RPM_NAME}-test/|rSH" \
    .
cp -r "${ci_data}/." "${rpm_tmp_dir}/SOURCES"

# Some CI environments do not have IPv6 connectivity, while vault.centos.org
# resolves to both IPv4 and IPv6 addresses. This result in occasional
# "Network is unreachable" errors. Workaround: disable IPv6 for yum.
if [ -w /etc/yum.conf ] && ! grep -q '^ip_resolve' /etc/yum.conf; then
	os::log::info "Disabling IPv6 for yum ..."
	echo 'ip_resolve=4' >> /etc/yum.conf
fi
# Just in case builddep isn't an installed plugin
dnf -y install 'dnf-command(builddep)'
dnf builddep -y "${OS_RPM_SPECFILE}" || true

rpmbuild -ba "${OS_RPM_SPECFILE}" \
    --define "_sourcedir ${rpm_tmp_dir}/SOURCES" \
    --define "_specdir ${rpm_tmp_dir}/SOURCES" \
    --define "_rpmdir ${rpm_tmp_dir}/RPMS" \
    --define "_srcrpmdir ${rpm_tmp_dir}/SRPMS" \
    --define "_builddir ${rpm_tmp_dir}/BUILD" \
    --define "version ${OS_RPM_VERSION}" \
    --define "release ${OS_RPM_RELEASE}" \
    --define "commit ${OS_GIT_COMMIT}" \
    --define 'debug_package %{nil}'

# migrate the rpm artifacts to the output directory, must be clean or move will fail
make clean
mkdir -p "${OS_OUTPUT}"

mkdir -p "${OS_OUTPUT_RPMPATH}"
mv -f "${rpm_tmp_dir}"/SRPMS/*src.rpm "${OS_OUTPUT_RPMPATH}"
mv -f "${rpm_tmp_dir}"/RPMS/*/*.rpm "${OS_OUTPUT_RPMPATH}"

mkdir -p "${OS_OUTPUT_RELEASEPATH}"
echo "${OS_GIT_COMMIT}" >"${OS_OUTPUT_RELEASEPATH}/.commit"

repo_path=${OS_OUTPUT_RPMPATH}
createrepo "${repo_path}"

echo "[${OS_RPM_NAME}-local-release]
baseurl = file://${repo_path}
gpgcheck = 0
name = Release from Local Source for ${OS_RPM_NAME}
enabled = 1
" >"${repo_path}/local-release.repo"

# DEPRECATED: preserve until jobs migrate to using local-release.repo
cp "${repo_path}/local-release.repo" "${repo_path}/cri-o-local-release.repo"

os::log::info "Repository file for \`yum\` or \`dnf\` placed at ${repo_path}/local-release.repo
Install it with:
$ mv '${repo_path}/local-release.repo' '/etc/yum.repos.d"
