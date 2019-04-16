let
  nixpkgs = builtins.fetchTarball {
    name = "nixos-unstable";
    url = "https://github.com/nixos/nixpkgs/archive/" +
      "03d6bb52b250d74d096e2286b02c51f595fa90ee.tar.gz";
    sha256 = "0bngjc4d190bgzhgxv9jszqkapx3ngdpj08rd2arz19ncggflnxv";
  };
  nixpkgsMuslSystemd = builtins.fetchTarball {
    name = "nixos-systemd-musl";
    url = "https://github.com/dtzWill/nixpkgs/archive/" +
      "13510d3dbe08e5bfc5454faf3fe543991dbf6e29.tar.gz";
    sha256 = "090zagbw39fa8fgwvzcmbcr204pxcfrq1rfw7ip5z44naj6gp7qb";
  };
in [ nixpkgs nixpkgsMuslSystemd ]
