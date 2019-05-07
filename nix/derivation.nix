{ flavor, ldflags ? "", revision,
  btrfs-progs, buildGoPackage, glibc, gpgme, libapparmor, libassuan,
  libgpgerror, libseccomp, libselinux, lvm2, pkgconfig, stdenv, systemd }:

buildGoPackage rec {
  project = "cri-o";
  name = "${project}-${revision}-${flavor}";

  goPackagePath = "github.com/${project}/${project}";
  src = ./..;

  outputs = [ "bin" "out" ];
  nativeBuildInputs = [ pkgconfig ];
  buildInputs = [ btrfs-progs gpgme libapparmor libassuan
                  libgpgerror libseccomp libselinux lvm2 systemd ] ++
                stdenv.lib.optionals (glibc != null) [ glibc glibc.static ];

  makeFlags = ''BUILDTAGS="apparmor seccomp selinux
    containers_image_ostree_stub"'';

  buildPhase = ''
    go build -tags ${makeFlags} -o bin/crio -buildmode=pie \
      -ldflags '-s -w ${ldflags}' ${goPackagePath}/cmd/crio
  '';
  installPhase = "install -Dm755 bin/crio $bin/bin/crio-${flavor}";

  meta = with import <nixpkgs/lib>; {
    homepage = https://cri-o.io;
    description = ''Open Container Initiative-based implementation of
                    the Kubernetes Container Runtime Interface'';
    license = licenses.asl20;
    platforms = platforms.linux;
  };
}
