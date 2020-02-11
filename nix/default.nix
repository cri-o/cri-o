{ revision ? "HEAD", system ? builtins.currentSystem }:
let
  nixpkgs = import ./nixpkgs.nix;
  glibcPkgs = import (builtins.head nixpkgs) {};
  muslPkgs = (import (builtins.head nixpkgs) {
    config = {
      packageOverrides = pkg: {
        go_1_13 = muslPkgs.go_1_12;
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
    cri-o-static-musl = (muslPkgs.cri-o.overrideAttrs(old: {
      name = "cri-o-x86_64-static-musl-${revision}";
      buildInputs = old.buildInputs ++ (with muslPkgs; [ systemd ]);
      src = ./..;
      EXTRA_LDFLAGS = ''-linkmode external -extldflags "-static"'';
    })).override {
      flavor = "-x86_64-static-musl";
      glibc = null;
      gpgme = (static muslPkgs.gpgme);
      libassuan = (static muslPkgs.libassuan);
      libgpgerror = (static muslPkgs.libgpgerror);
      libseccomp = (static muslPkgs.libseccomp);
      lvm2 = (patchLvm2 (static muslPkgs.lvm2));
    };
    cri-o-static-glibc = (glibcPkgs.cri-o.overrideAttrs(old: {
      name = "cri-o-x86_64-static-glibc-${revision}";
      buildInputs = old.buildInputs ++ (with glibcPkgs; [ systemd ]);
      src = ./..;
      EXTRA_LDFLAGS = ''-linkmode external -extldflags "-static -lm"'';
    })).override {
      flavor = "-x86_64-static-glibc";
      gpgme = (static glibcPkgs.gpgme);
      libassuan = (static glibcPkgs.libassuan);
      libgpgerror = (static glibcPkgs.libgpgerror);
      libseccomp = (static glibcPkgs.libseccomp);
      lvm2 = (patchLvm2 (static glibcPkgs.lvm2));
    };
  };
in self
