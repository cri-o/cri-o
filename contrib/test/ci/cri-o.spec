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

%if ! 0%{?centos} && 0%{?rhel}
# Golang minor version
%global gominver 10
%define gobuild(o:) scl enable go-toolset-1.%{gominver} -- go build -buildmode pie -compiler gc -tags="rpm_crashtraceback no_openssl ${BUILDTAGS:-}" -ldflags "${LDFLAGS:-} -B 0x$(head -c20 /dev/urandom|od -An -tx1|tr -d ' \\n') -extldflags '%__global_ldflags'" -a -v -x %{?**};
%else
%define gobuild(o:) go build -buildmode pie -compiler gc -tags="rpm_crashtraceback no_openssl ${BUILDTAGS:-}" -ldflags "${LDFLAGS:-} -B 0x$(head -c20 /dev/urandom|od -An -tx1|tr -d ' \\n') -extldflags '%__global_ldflags'" -a -v -x %{?**};
%endif # !centos

%global provider github
%global provider_tld com
%global project cri-o
%global repo cri-o
# https://github.com/kubernetes-sigs/cri-o
%global provider_prefix %{provider}.%{provider_tld}/%{project}/%{repo}
%global import_path %{provider_prefix}
%global git0 https://%{import_path}
#%%global commit0 ee2e7485ffe9c6d8932ec6acb0adcb7a0a55c253

%global service_name crio

Name: %{repo}
Version: 1.13
Release: 1.ci%{?dist}
Summary: Kubernetes Container Runtime Interface for OCI-based containers
License: ASL 2.0
URL: %{git0}
Source0: %{name}-test.tar.gz
Source3: %{service_name}-network.sysconfig
Source4: %{service_name}-storage.sysconfig
Source5: %{service_name}-metrics.sysconfig
Source6: %{service_name}.service
%if ! 0%{?centos} && 0%{?rhel}
BuildRequires: go-toolset-1.%{gominver}
%else
BuildRequires: %{?go_compiler:compiler(go-compiler)}%{!?go_compiler:golang}
BuildRequires: make
%endif # !centos
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
Requires: containernetworking-plugins >= 0.7.2-1

%description
%{summary}

%{?enable_gotoolset1%{?gominver}}

%prep
%setup -qn %{name}-test
cp %{SOURCE6} contrib/systemd/.
sed -i '/strip/d' pause/Makefile
sed -i 's/install.config: crio.conf/install.config:/' Makefile
sed -i 's/install.bin: binaries/install.bin:/' Makefile
sed -i 's/\.gopathok//' Makefile
sed -i 's/go test/$(GO) test/' Makefile
sed -i 's/%{version}/%{version}-%{release}/' version/version.go
sed -i 's/\/local//' contrib/systemd/%{service_name}.service

%build
mkdir _output
pushd _output
mkdir -p src/%{provider}.%{provider_tld}/{%{project},opencontainers}
ln -s $(dirs +1 -l) src/%{import_path}
popd

ln -s vendor src
export GOPATH=$(pwd)/_output:$(pwd)
export BUILDTAGS="selinux seccomp exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_ostree_stub"
# build crio
%gobuild -o bin/%{service_name} %{import_path}/cmd/%{service_name}

# build conmon
%gobuild -o bin/crio-config %{import_path}/cmd/crio-config
pushd conmon && ../bin/crio-config
%{__make} all
popd

# build pause and docs
%{__make} GO_MD2MAN=%{_bindir}/go-md2man bin/pause docs

%install
./bin/%{service_name} \
      --selinux \
      --cgroup-manager "systemd" \
      --storage-driver "overlay" \
      --conmon "%{_libexecdir}/%{service_name}/conmon" \
      --cni-plugin-dir "%{_libexecdir}/cni" \
      --default-mounts "%{_datadir}/rhel/secrets:/run/secrets" \
      --storage-opt "overlay.override_kernel_check=1" \
      --file-locking=false config > ./%{service_name}.conf

# install conf files
install -dp %{buildroot}%{_sysconfdir}/cni/net.d
install -p -m 644 contrib/cni/10-crio-bridge.conf %{buildroot}%{_sysconfdir}/cni/net.d/100-crio-bridge.conf
install -p -m 644 contrib/cni/99-loopback.conf %{buildroot}%{_sysconfdir}/cni/net.d/200-loopback.conf

install -dp %{buildroot}%{_sysconfdir}/sysconfig
install -p -m 644 contrib/sysconfig/%{service_name} %{buildroot}%{_sysconfdir}/sysconfig/%{service_name}
install -p -m 644 %{SOURCE3} %{buildroot}%{_sysconfdir}/sysconfig/%{service_name}-network
install -p -m 644 %{SOURCE4} %{buildroot}%{_sysconfdir}/sysconfig/%{service_name}-storage
install -p -m 644 %{SOURCE5} %{buildroot}%{_sysconfdir}/sysconfig/%{service_name}-metrics

make PREFIX=%{buildroot}%{_usr} DESTDIR=%{buildroot} \
            install.bin \
            install.completions \
            install.config \
            install.man \
            install.systemd

install -dp %{buildroot}%{_sharedstatedir}/containers

%check
%if 0%{?with_check}
export GOPATH=%{buildroot}/%{gopath}:$(pwd)/Godeps/_workspace:%{gopath}
%endif

%post
%systemd_post %{service_name}

%preun
%systemd_preun %{service_name}

%postun
%systemd_postun_with_restart %{service_name}

#define license tag if not already defined
%{!?_licensedir:%global license %doc}

%files
%license LICENSE
%doc README.md
%{_bindir}/%{service_name}
%{_mandir}/man5/%{service_name}.conf.5*
%{_mandir}/man8/%{service_name}.8*
%dir %{_sysconfdir}/%{service_name}
%config(noreplace) %{_sysconfdir}/%{service_name}/%{service_name}.conf
%config(noreplace) %{_sysconfdir}/%{service_name}/seccomp.json
%config(noreplace) %{_sysconfdir}/sysconfig/%{service_name}
%config(noreplace) %{_sysconfdir}/sysconfig/%{service_name}-storage
%config(noreplace) %{_sysconfdir}/sysconfig/%{service_name}-network
%config(noreplace) %{_sysconfdir}/sysconfig/%{service_name}-metrics
%config(noreplace) %{_sysconfdir}/cni/net.d/100-%{service_name}-bridge.conf
%config(noreplace) %{_sysconfdir}/cni/net.d/200-loopback.conf
%config(noreplace) %{_sysconfdir}/crictl.yaml
%dir %{_libexecdir}/%{service_name}
%{_libexecdir}/%{service_name}/conmon
%{_libexecdir}/%{service_name}/pause
%{_unitdir}/%{service_name}.service
%{_unitdir}/%{name}.service
%{_unitdir}/%{service_name}-shutdown.service
%dir %{_sharedstatedir}/containers
%dir %{_datadir}/oci-umount
%dir %{_datadir}/oci-umount/oci-umount.d
%{_datadir}/oci-umount/oci-umount.d/%{service_name}-umount.conf

%changelog
* Wed May 08 2019 Lokesh Mandvekar <lsm5@fedoraproject.org> - 1.13-1.ci
- first ci spec
- NO NEED TO UPDATE CHANGELOG