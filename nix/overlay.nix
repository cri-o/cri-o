let
  static = import ./static.nix;
in
self: super:
{
  gpgme = (static super.gpgme);
  libassuan = (static super.libassuan);
  libgpgerror = (static super.libgpgerror);
  libseccomp = (static super.libseccomp);
  gnupg = super.gnupg.override {
    libusb1 = null;
    pcsclite = null;
    enableMinimal = true;
    guiSupport = false;
  };
}
