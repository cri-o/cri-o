let
  nixpkgs = builtins.fetchTarball {
    name = "nixos-unstable";
    url = "https://github.com/nixos/nixpkgs/archive/" +
      "221274e1552fdb84e8caf0831dbd9140b111131e.tar.gz";
    sha256 = "1ic9d9c27qpcrx5n5dn1mak3hrbbdq1is24p11rqrj9xas79lf89";
  };
  nixpkgsMuslSystemd = builtins.fetchTarball {
    name = "nixos-systemd-musl";
    url = "https://github.com/dtzWill/nixpkgs/archive/" +
      "13510d3dbe08e5bfc5454faf3fe543991dbf6e29.tar.gz";
    sha256 = "090zagbw39fa8fgwvzcmbcr204pxcfrq1rfw7ip5z44naj6gp7qb";
  };
in [ nixpkgs nixpkgsMuslSystemd ]
