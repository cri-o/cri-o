let
  static = import ./static.nix;
in
self: super:
{
  gpgme = (static super.gpgme).overrideAttrs (x: {
    # Drop the --enable-fixed-path:
    # https://github.com/nixos/nixpkgs/blob/9a79bc99/pkgs/development/libraries/gpgme/default.nix#L94
    configureFlags = self.lib.lists.remove "--enable-fixed-path=${self.gnupg}/bin" x.configureFlags;
  });
  libassuan = (static super.libassuan);
  libgpg-error = (static super.libgpg-error);
  libseccomp = (static super.libseccomp);
  gnupg = super.gnupg.override {
    libusb1 = null;
    pcsclite = null;
    enableMinimal = true;
    guiSupport = false;
  };
}
