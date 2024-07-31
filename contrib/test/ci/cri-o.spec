# This spec is meant for CI testing only.
# Any changes here will NOT automagically land in your distro's rpm after
# an update.

%global with_debug 1

%if 0%{?with_debug}
%global _find_debuginfo_dwz_opts %{nil}
%global _dwz_low_mem_die_limit 0
%else
%global debug_package %{nil}
%endif

%define gobuild(o:) go build -buildmode pie -compiler gc -tags="rpm_crashtraceback no_openssl ${BUILDTAGS:-}" -ldflags "${LDFLAGS:-} -B 0x$(head -c20 /dev/urandom|od -An -tx1|tr -d ' \\n') -extldflags '%__global_ldflags'" -a -v -x %{?**};

%global provider github
%global provider_tld com
%global project cri-o
%global repo cri-o
# https://github.com/cri-o/cri-o
%global provider_prefix %{provider}.%{provider_tld}/%{project}/%{repo}
%global import_path %{provider_prefix}
%global git0 https://%{import_path}
#%%global commit0 ee2e7485ffe9c6d8932ec6acb0adcb7a0a55c253

%global service_name crio

Name: %{repo}
Version: 1.26.0
Release: 1.ci%{?dist}
Summary: Kubernetes Container Runtime Interface for OCI-based containers
License: ASL 2.0
URL: %{git0}
Source0: %{name}-test.tar.gz
# Assume pre-installed golang (which is the case in our CI)
BuildRequires: make
BuildRequires: git
BuildRequires: glib2-devel
BuildRequires: glibc-static
BuildRequires: go-md2man
BuildRequires: gpgme-devel
BuildRequires: libassuan-devel
BuildRequires: libseccomp-devel
BuildRequires: pkgconfig(systemd)
Requires(pre): container-selinux
Requires: containers-common >= 1:0.1.24-3
Requires: runc > 1.0.0-57
Obsoletes: ocid <= 0.3
Provides: ocid = %{version}-%{release}
Provides: %{service_name} = %{version}-%{release}
Requires: containernetworking-plugins >= 0.7.5-1
Requires: conmon

%description
%{summary}

%{?enable_gotoolset1%{?gominver}}

%prep
%setup -qn %{name}-test
sed -i 's/install.config: crio.conf/install.config:/' Makefile
sed -i 's/install.bin: binaries/install.bin:/' Makefile
sed -i 's/\.gopathok//' Makefile
sed -i 's/go test/$(GO) test/' Makefile
sed -i 's/%{version}/%{version}-%{release}/' internal/version/version.go
sed -i 's/\/local//' contrib/systemd/%{service_name}.service

%build
mkdir _output
pushd _output
mkdir -p src/%{provider}.%{provider_tld}/{%{project},opencontainers}
ln -s $(dirs +1 -l) src/%{import_path}
popd

ln -s vendor src
export GOPATH=$(pwd)/_output:$(pwd)
export BUILDTAGS="selinux seccomp exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_ostree_stub containers_image_openpgp"
make bin/crio bin/pinns

# build docs
make GO_MD2MAN=go-md2man docs

%install
./bin/%{service_name} \
      --selinux \
      --cgroup-manager "systemd" \
      --storage-driver "overlay" \
      --conmon "%{_bindir}/conmon" \
      --cni-plugin-dir "%{_libexecdir}/cni" \
      --storage-opt "overlay.override_kernel_check=1" \
      config > ./%{service_name}.conf

# install conf files
install -dp %{buildroot}%{_sysconfdir}/cni/net.d
mkdir -p %{buildroot}%{_sysconfdir}/%{service_name}
install -p -m 644 ./%{service_name}.conf %{buildroot}%{_sysconfdir}/%{service_name}/%{service_name}.conf
install -p -m 644 contrib/cni/10-crio-bridge.conflist %{buildroot}%{_sysconfdir}/cni/net.d/100-crio-bridge.conf
install -p -m 644 contrib/cni/99-loopback.conflist %{buildroot}%{_sysconfdir}/cni/net.d/200-loopback.conf

make PREFIX=%{buildroot}%{_usr} DESTDIR=%{buildroot} \
            install.bin \
            install.completions \
            install.config \
            install.man \
            install.systemd

install -dp %{buildroot}%{_sharedstatedir}/containers

#rm %{_sysconfdir}/%{service_name}/%{service_name}.conf.d/00-default.conf

%check
%if 0%{?with_check}
export GOPATH=%{buildroot}/%{gopath}:$(pwd)/Godeps/_workspace:%{gopath}
%endif

%post
ln -sf %{_unitdir}/%{service_name}.service %{_unitdir}/%{repo}.service
%systemd_post %{service_name}

%preun
rm -f %{_unitdir}/%{repo}.service
%systemd_preun %{service_name}

%postun
%systemd_postun_with_restart %{service_name}

#define license tag if not already defined
%{!?_licensedir:%global license %doc}

%files
%license LICENSE
%doc README.md
%{_bindir}/%{service_name}
%{_bindir}/pinns
%{_mandir}/man5/%{service_name}.conf.5*
%{_mandir}/man5/%{service_name}.conf.d.5*
%{_mandir}/man8/%{service_name}*.8*
%dir %{_sysconfdir}/%{service_name}
%config(noreplace) %{_sysconfdir}/%{service_name}/%{service_name}.conf
%config(noreplace) %{_sysconfdir}/cni/net.d/100-%{service_name}-bridge.conf
%config(noreplace) %{_sysconfdir}/cni/net.d/200-loopback.conf
%config(noreplace) %{_sysconfdir}/crictl.yaml
%{_unitdir}/%{service_name}.service
%dir %{_sharedstatedir}/containers
%dir %{_datadir}/oci-umount
%dir %{_datadir}/oci-umount/oci-umount.d
%{_datadir}/oci-umount/oci-umount.d/%{service_name}-umount.conf
%{_unitdir}/%{service_name}-wipe.service
%{_datadir}/bash-completion/completions/%{service_name}
%{_datadir}/fish/completions/%{service_name}.fish
%{_datadir}/zsh/site-functions/_%{service_name}



%changelog
* Wed May 08 2019 Lokesh Mandvekar <lsm5@fedoraproject.org> - 1.13-1.ci
- first ci spec
- NO NEED TO UPDATE CHANGELOG
