{ system ? builtins.currentSystem }:
let
  pkgs = (import ./nixpkgs.nix {
    config = {
      packageOverrides = pkg: {
        gpgme = (static pkg.gpgme);
        libassuan = (static pkg.libassuan);
        libgpgerror = (static pkg.libgpgerror);
        libseccomp = (static pkg.libseccomp);
      };
    };
  });

  static = pkg: pkg.overrideAttrs(x: {
    configureFlags = (x.configureFlags or []) ++
      [ "--without-shared" "--disable-shared" ];
    dontDisableStatic = true;
    enableSharedExecutables = false;
    enableStatic = true;
  });

  self = with pkgs; {
    cri-o-static = (cri-o.overrideAttrs(x: {
      name = "cri-o-static";
      src = ./..;
      doCheck = false;
      buildInputs = [
        glibc
        glibc.static
        gpgme
        libapparmor
        libassuan
        libgpgerror
        libseccomp
        libselinux
      ];
      prePatch = ''
        export LDFLAGS='-static-libgcc -static'
        export EXTRA_LDFLAGS='-linkmode external -extldflags "-static -lm"'
        export BUILDTAGS='static netgo apparmor selinux seccomp exclude_graphdriver_btrfs exclude_graphdriver_devicemapper'
      '';
    }));
  };
in self
