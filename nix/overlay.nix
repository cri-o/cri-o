let
  static = import ./static.nix;
in
self: super:
{
  zlib = super.zlib.overrideAttrs (old: self.lib.optionalAttrs self.stdenv.hostPlatform.isS390 {
    # Disable s390x vectorized CRC32 to fix cross-compilation.
    # zlib >= 1.3.2 crc32_vx.c requires -mvx but the build does not pass it.
    postPatch = (old.postPatch or "") + ''
      substituteInPlace configure \
        --replace-warn 'enable_crcvx=1' 'enable_crcvx=0'
    '';
  });
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
