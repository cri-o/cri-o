let
  nixpkgs = builtins.fetchTarball {
    name = "nixos-unstable";
    url = "https://github.com/nixos/nixpkgs/archive/" +
      "3d26aeea173e2bc979dea1f5cbcbfbb9b5f73a47.tar.gz";
    sha256 = "1is048xxm69qirgrq2iws22dnj98gzkz8i8m00js6kfkl4kzigkp";
  };
  nixpkgsMuslSystemd = builtins.fetchTarball {
    name = "nixos-systemd-musl";
    url = "https://github.com/dtzWill/nixpkgs/archive/" +
      "13510d3dbe08e5bfc5454faf3fe543991dbf6e29.tar.gz";
    sha256 = "090zagbw39fa8fgwvzcmbcr204pxcfrq1rfw7ip5z44naj6gp7qb";
  };
in [ nixpkgs nixpkgsMuslSystemd ]
