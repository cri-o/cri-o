{ system ? builtins.currentSystem }:
let
  pkgs = (import ./nixpkgs.nix {
    config = {
      packageOverrides = pkg: {
        gpgme = (static pkg.gpgme);
        libassuan = (static pkg.libassuan);
        libgpgerror = (static pkg.libgpgerror);
        libseccomp = (static pkg.libseccomp);
        go_1_14 = pkg.go_1_14.overrideAttrs (old: {
          prePatch = ''
            sed -i 's/TestChown/testChown/' src/os/os_unix_test.go
            sed -i 's/TestFileChown/testFileChown/' src/os/os_unix_test.go
            sed -i 's/TestLchown/testLchown/' src/os/os_unix_test.go
            sed -i 's/TestTicker/testTicker/' src/time/tick_test.go
          '' + old.prePatch;
        });
      };
    };
  });

  static = pkg: pkg.overrideAttrs(x: {
    doCheck = false;
    configureFlags = (x.configureFlags or []) ++ [
      "--without-shared"
      "--disable-shared"
    ];
    dontDisableStatic = true;
    enableSharedExecutables = false;
    enableStatic = true;
  });

  self = with pkgs; buildGoModule rec {
    name = "cri-o";
    src = ./..;
    vendorSha256 = null;
    doCheck = false;
    enableParallelBuilding = true;
    outputs = [ "out" ];
    nativeBuildInputs = [ bash git go-md2man installShellFiles makeWrapper pkg-config which ];
    buildInputs = [ glibc glibc.static gpgme libassuan libgpgerror libseccomp libapparmor libselinux ];
    prePatch = ''
      export CFLAGS='-static'
      export LDFLAGS='-s -w -static-libgcc -static'
      export EXTRA_LDFLAGS='-s -w -linkmode external -extldflags "-static -lm"'
      export BUILDTAGS='static netgo exclude_graphdriver_btrfs exclude_graphdriver_devicemapper seccomp apparmor selinux'
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
  };
in self
