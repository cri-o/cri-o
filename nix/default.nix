(import ./nixpkgs.nix {
  overlays = [ (import ./overlay.nix) ];
}).callPackage ./derivation.nix
{ }
