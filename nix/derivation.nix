{ stdenv
, pkgs
}:
with pkgs; buildGo125Module /* use go 1.25 */ {
  name = "cri-o";
  # Use Pure to avoid exuding the .git directory
  src = nix-gitignore.gitignoreSourcePure [ ../.gitignore ] ./..;
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
  buildInputs = lib.optionals (!stdenv.hostPlatform.isMusl) [
    glibc
    glibc.static
  ] ++ [
    gpgme
    libapparmor
    libassuan
    libgpg-error
    libseccomp
    libselinux
  ];
  prePatch = ''
    export CFLAGS='-static -pthread'
    export LDFLAGS='-s -w -static-libgcc -static'
    export EXTRA_LDFLAGS='-s -w -linkmode external -extldflags "-static -lm"'
    export BUILDTAGS='static netgo osusergo exclude_graphdriver_btrfs seccomp apparmor selinux'
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
