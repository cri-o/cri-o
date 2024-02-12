{ pkgs }:
with pkgs; buildGo122Module {
  name = "cri-o";
  src = ./..;
  vendorHash = null;
  doCheck = false;
  enableParallelBuilding = true;
  outputs = [ "out" ];
  nativeBuildInputs = with buildPackages; [
    bash
    gitMinimal
    go-md2man
    installShellFiles
    makeWrapper
    pkg-config
    which
  ];
  buildInputs = [
    glibc
    glibc.static
    gpgme
    libassuan
    libgpgerror
    libseccomp
    libapparmor
    libselinux
  ];
  prePatch = ''
    export CFLAGS='-static -pthread'
    export LDFLAGS='-s -w -static-libgcc -static'
    export EXTRA_LDFLAGS='-s -w -linkmode external -extldflags "-static -lm"'
    export BUILDTAGS='static netgo osusergo exclude_graphdriver_btrfs exclude_graphdriver_devicemapper seccomp apparmor selinux'
    export CGO_ENABLED=1
    export CGO_LDFLAGS='-lgpgme -lassuan -lgpg-error'
    export SOURCE_DATE_EPOCH=0
  '';
  buildPhase = ''
    make binaries
  '';
  installPhase = ''
    install -Dm755 bin/crio $out/bin/crio
    install -Dm755 bin/pinns $out/bin/pinns
  '';
}
