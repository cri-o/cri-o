let
  json = builtins.fromJSON (builtins.readFile ./nixpkgs.json);
  nixpkgs = import (builtins.fetchTarball {
    name = "nixos-unstable";
    url = "${json.url}/tarball/${json.rev}";
    inherit (json) sha256;
  });
in nixpkgs
