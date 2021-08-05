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

# Golang minor version
%if 0%{?rhel} && ! 0%{?fedora}
%define gobuild(o:) go build -buildmode pie -compiler gc -tags="rpm_crashtraceback libtrust_openssl ${BUILDTAGS:-}" -ldflags "${LDFLAGS:-} -compressdwarf=false -B 0x$(head -c20 /dev/urandom|od -An -tx1|tr -d ' \\n') -extldflags '%__global_ldflags'" -a -v -x %{?**};
%else
%define gobuild(o:) go build -buildmode pie -compiler gc -tags="rpm_crashtraceback ${BUILDTAGS:-}" -ldflags "${LDFLAGS:-} -B 0x$(head -c20 /dev/urandom|od -An -tx1|tr -d ' \\n') -extldflags '%__global_ldflags'" -a -v -x %{?**};
%endif

%global provider github
%global provider_tld com
%global project cri-o
%global repo cri-o
# https://github.com/cri-o/cri-o
%global import_path %{provider}.%{provider_tld}/%{project}/%{repo}
%global commit0 1cf63c70073ac4469d12cf15d1740eb18cde8c93
%global shortcommit0 %(c=%{commit0}; echo ${c:0:7})
%global git0 https://%{import_path}

%global service_name crio

Name: %{repo}
Version: 1.22.0
Release: 1.ci.git%{shortcommit0}%{?dist}
Summary: Kubernetes Container Runtime Interface for OCI-based containers
License: ASL 2.0
URL: %{git0}
Source0: %{name}-test.tar.gz
Source6: %{service_name}.service
BuildRequires: golang
BuildRequires: git
BuildRequires: glib2-devel
BuildRequires: glibc-static
BuildRequires: go-md2man
BuildRequires: gpgme-devel
BuildRequires: libassuan-devel
BuildRequires: libseccomp-devel
BuildRequires: pkgconfig(systemd)
Requires(pre): container-selinux
Requires: skopeo-containers >= 1:0.1.40-1
Requires: runc >= 1.0.0-61.rc8
Obsoletes: ocid <= 0.3
Provides: ocid = %{version}-%{release}
Provides: %{service_name} = %{version}-%{release}
Requires: containernetworking-plugins >= 0.8.2-3
Requires: conmon >= 2.0.2-2

%description
%{summary}

%prep
%setup -qn %{name}-test
cp %{SOURCE6} contrib/systemd/.
sed -i 's/install.config: crio.conf/install.config:/' Makefile
sed -i 's/install.bin: binaries/install.bin:/' Makefile
sed -i 's/\.gopathok//' Makefile
sed -i 's/go test/$(GO) test/' Makefile
sed -i 's/%{version}/%{version}-%{release}/' internal/version/version.go
sed -i 's/\/local//' contrib/systemd/%{service_name}.service
sed -i 's/\/local//' contrib/systemd/%{service_name}-wipe.service

%build
mkdir _output
pushd _output
mkdir -p src/%{provider}.%{provider_tld}/{%{project},opencontainers}
ln -s $(dirs +1 -l) src/%{import_path}
popd

ln -s vendor src
export GOPATH=$(pwd)/_output:$(pwd)
export BUILDTAGS="selinux seccomp exclude_graphdriver_devicemapper exclude_graphdriver_btrfs containers_image_ostree_stub"
export GO111MODULE=off
# https://bugzilla.redhat.com/show_bug.cgi?id=1825623
export VERSION=%{version}
# build crio
%gobuild -o bin/%{service_name} %{import_path}/cmd/%{service_name}

# build crio-status
%gobuild -o bin/crio-status %{import_path}/cmd/crio-status

# build pinns and docs
%{__make} bin/pinns
%if 0%{?rhel} <= 7 && ! 0%{?fedora}
GO_MD2MAN=go-md2man GO="scl enable go-toolset-1.13 -- go" %{__make} docs
%else
GO_MD2MAN=go-md2man %{__make} docs
%endif

%install
./bin/%{service_name} \
      --selinux \
      --cgroup-manager "systemd" \
      --storage-driver "overlay" \
      --conmon "%{_libexecdir}/crio/conmon" \
      --cni-plugin-dir "%{_libexecdir}/cni" \
      --storage-opt "overlay.override_kernel_check=1" \
      --metrics-port 9537 \
      --enable-metrics \
      config > %{service_name}.conf

# install conf files
install -dp %{buildroot}%{_sysconfdir}/cni/net.d
install -p -m 644 contrib/cni/10-crio-bridge.conf %{buildroot}%{_sysconfdir}/cni/net.d/100-crio-bridge.conf
install -p -m 644 contrib/cni/99-loopback.conf %{buildroot}%{_sysconfdir}/cni/net.d/200-loopback.conf
make PREFIX=%{buildroot}%{_prefix} DESTDIR=%{buildroot} \
            install.bin-nobuild \
            install.completions \
            install.config-nobuild \
            install.man-nobuild \
            install.systemd

# install seccomp.json
install -dp %{buildroot}%{_sharedstatedir}/containers
install -dp %{buildroot}%{_sharedstatedir}/cni/bin
install -dp %{buildroot}%{_sysconfdir}/kubernetes/cni/net.d
install -dp %{buildroot}%{_datadir}/containers/oci/hooks.d
install -dp %{buildroot}/opt/cni/bin

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
%{_bindir}/%{service_name}-status
%{_bindir}/pinns
%{_mandir}/man5/%{service_name}.conf*5*
%{_mandir}/man8/%{service_name}*.8*
%dir %{_sysconfdir}/%{service_name}
%config(noreplace) %{_sysconfdir}/%{service_name}/%{service_name}.conf
%config(noreplace) %{_sysconfdir}/%{service_name}/seccomp.json
%config(noreplace) %{_sysconfdir}/cni/net.d/100-%{service_name}-bridge.conf
%config(noreplace) %{_sysconfdir}/cni/net.d/200-loopback.conf
%config(noreplace) %{_sysconfdir}/crictl.yaml
%{_unitdir}/%{service_name}.service
%{_unitdir}/%{name}.service
%{_unitdir}/%{service_name}-shutdown.service
%{_unitdir}/%{service_name}-wipe.service
%dir %{_sharedstatedir}/containers
%dir %{_sharedstatedir}/cni
%dir %{_sharedstatedir}/cni/bin
%dir %{_sysconfdir}/kubernetes
%dir %{_sysconfdir}/kubernetes/cni
%dir %{_sysconfdir}/kubernetes/cni/net.d
%dir %{_datadir}/containers
%dir %{_datadir}/containers/oci
%dir %{_datadir}/containers/oci/hooks.d
%dir /opt/cni
%dir /opt/cni/bin
%dir %{_datadir}/oci-umount
%dir %{_datadir}/oci-umount/oci-umount.d
%{_datadir}/oci-umount/oci-umount.d/%{service_name}-umount.conf
%{_datadir}/bash-completion/completions/%{service_name}*
%{_datadir}/fish/completions/%{service_name}*.fish
%{_datadir}/zsh/site-functions/_%{service_name}*

%changelog
* Wed May 08 2019 Lokesh Mandvekar <lsm5@fedoraproject.org> - 1.13-1.ci
- first ci spec
- NO NEED TO UPDATE CHANGELOG
