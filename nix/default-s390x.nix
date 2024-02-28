(import ./nixpkgs.nix {
  crossSystem = {
    config = "s390x-unknown-linux-gnu";
  };
  overlays = [ (import ./overlay.nix) ];
}).callPackage ./derivation.nix
{ }
