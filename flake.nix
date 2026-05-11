{
  description = "CRI-O: OCI-based implementation of Kubernetes Container Runtime Interface";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixpkgs-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      supportedSystems = [ "x86_64-linux" "aarch64-linux" ];

      forAllSystems = nixpkgs.lib.genAttrs supportedSystems;

      crossTargets = {
        amd64 = {
          config = "x86_64-unknown-linux-gnu";
        };
        arm64 = {
          config = "aarch64-unknown-linux-gnu";
        };
        ppc64le = {
          config = "powerpc64le-unknown-linux-gnu";
        };
        s390x = {
          config = "s390x-unknown-linux-musl";
        };
      };

      mkCriO =
        system: crossSystem:
        let
          pkgs = import nixpkgs {
            inherit system;
            crossSystem = crossSystem;
            overlays = [ (import ./nix/overlay.nix) ];
          };
          rev = self.rev or self.dirtyRev or "unknown";
        in
        pkgs.callPackage ./nix/derivation.nix { gitCommit = rev; };

      # Map target config to native nix system string
      configToSystem = {
        "x86_64-unknown-linux-gnu" = "x86_64-linux";
        "aarch64-unknown-linux-gnu" = "aarch64-linux";
      };
    in
    {
      packages = forAllSystems (
        system:
        let
          native = mkCriO system null;
        in
        {
          default = native;
          crio = native;
        }
        // nixpkgs.lib.mapAttrs' (
          arch: crossSystem:
          nixpkgs.lib.nameValuePair "crio-${arch}" (
            if (configToSystem.${crossSystem.config} or null) == system then
              native
            else
              mkCriO system crossSystem
          )
        ) crossTargets
      );
    };
}
