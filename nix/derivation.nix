{ pkgs }:
with pkgs; buildGo120Module {
  name = "cri-o";
  src = ./..;
  vendorSha256 = null;
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
  '';
  buildPhase = ''
    patchShebangs .
    make bin/crio
    make bin/crio-status
    make bin/pinns
  '';
  installPhase = ''
    install -Dm755 bin/crio $out/bin/crio
    install -Dm755 bin/crio-status $out/bin/crio-status
    install -Dm755 bin/pinns $out/bin/pinns
  '';
}
