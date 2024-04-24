(import ./nixpkgs.nix {
  crossSystem = {
    # TODO: Switch back to glibc when
    # https://github.com/NixOS/nixpkgs/issues/306473
    # is resolved.
    config = "s390x-unknown-linux-musl";
  };
  overlays = [ (import ./overlay.nix) ];
}).callPackage ./derivation.nix
{ }
