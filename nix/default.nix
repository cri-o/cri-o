{ revision ? "HEAD", system ? builtins.currentSystem }:
let
  nixpkgs = import ./nixpkgs.nix;

  glibcPkgs = import (builtins.head nixpkgs) {
    config = { packageOverrides = pkg: { go_1_11 = glibcPkgs.go_1_12; }; };
  };
  glibcCallPackage = glibcPkgs.lib.callPackageWith
                     (glibcPkgs // glibcPkgs.xlibs // self);

  muslPkgs = (import (builtins.head nixpkgs) {
    config = {
      packageOverrides = pkg: {
        go_bootstrap = pkg.go_bootstrap.overrideAttrs (old: {
          installPhase = ''
            sed -i 's/TestChown/testChown/' src/os/os_unix_test.go
          '' + old.installPhase;
        });
        go_1_11 = muslPkgs.go_1_12;
        go_1_12 = pkg.go_1_12.overrideAttrs (old: {
          prePatch = ''
            sed -i 's/TestChown/testChown/' src/os/os_unix_test.go
            sed -i 's/TestFileChown/testFileChown/' src/os/os_unix_test.go
            sed -i 's/TestLchown/testLchown/' src/os/os_unix_test.go
            sed -i 's/TestCoverageUsesAtomicModeForRace/testCoverageUsesAtomicModeForRace/' src/cmd/go/go_test.go
            sed -i 's/TestTestRaceInstall/testTestRaceInstall/' src/cmd/go/go_test.go
            sed -i 's/TestGoTestRaceFailures/testGoTestRaceFailures/' src/cmd/go/go_test.go
            sed -i '/func cmdtest/a return' src/cmd/dist/test.go
          '' + old.prePatch;
        });
        gnupg = glibcPkgs.gnupg;
        systemd = (import (builtins.elemAt nixpkgs 1) {}).systemd;
      };
    };
  }).pkgsMusl;
  muslCallPackage = muslPkgs.lib.callPackageWith
                    (muslPkgs // muslPkgs.xlibs // self);

  static = pkg: pkg.overrideAttrs(old: {
    configureFlags = (old.configureFlags or []) ++
      [ "--without-shared" "--disable-shared" ];
    dontDisableStatic = true;
    enableSharedExecutables = false;
    enableStatic = true;
  });

  patchLvm2 = pkg: pkg.overrideAttrs(old: {
    configureFlags = [
      "--disable-cmdlib" "--disable-readline" "--disable-udev_rules"
      "--disable-udev_sync" "--enable-pkgconfig" "--enable-static_link"
    ];
    preConfigure = old.preConfigure + ''
      substituteInPlace libdm/Makefile.in --replace \
        SUBDIRS=dm-tools SUBDIRS=
      substituteInPlace tools/Makefile.in --replace \
        "TARGETS += lvm.static" ""
      substituteInPlace tools/Makefile.in --replace \
        "INSTALL_LVM_TARGETS += install_tools_static" ""
    '';
    postInstall = "";
  });

  self = {
    cri-o-static-musl = muslCallPackage ./derivation.nix {
      flavor = "x86_64-static-musl";
      ldflags = ''-linkmode external -extldflags "-static"'';
      revision = revision;

      glibc = null;
      gpgme = (static muslPkgs.gpgme);
      libassuan = (static muslPkgs.libassuan);
      libgpgerror = (static muslPkgs.libgpgerror);
      libseccomp = (static muslPkgs.libseccomp);
      lvm2 = (patchLvm2 (static muslPkgs.lvm2));
    };
    cri-o-static-glibc = glibcCallPackage ./derivation.nix {
      flavor = "x86_64-static-glibc";
      ldflags = ''-linkmode external -extldflags "-static -lm"'';
      revision = revision;

      gpgme = (static glibcPkgs.gpgme);
      libassuan = (static glibcPkgs.libassuan);
      libgpgerror = (static glibcPkgs.libgpgerror);
      libseccomp = (static glibcPkgs.libseccomp);
      lvm2 = (patchLvm2 (static glibcPkgs.lvm2));
    };
  };
in self
