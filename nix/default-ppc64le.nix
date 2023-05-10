let
  static = import ./static.nix;

  pkgs = (import ./nixpkgs.nix {
    crossSystem = {
      config = "powerpc64le-unknown-linux-gnu";
    };
    config = {
      packageOverrides = pkg: {
        gpgme = (static pkg.gpgme);
        libassuan = (static pkg.libassuan);
        libgpgerror = (static pkg.libgpgerror);
        libseccomp = (static pkg.libseccomp);
        gnupg = pkg.gnupg.override {
          libusb1 = null;
          pcsclite = null;
          enableMinimal = true;
          guiSupport = false;
        };
      };
    };
  });

  self = import ./derivation.nix { inherit pkgs; };
in
self
