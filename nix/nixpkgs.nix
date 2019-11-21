let
  nixpkgs = builtins.fetchTarball {
    name = "nixos-unstable";
    url = "https://github.com/nixos/nixpkgs/archive/" +
      "f28fad5e2fe777534f1c2719a40e69812085dfe5.tar.gz";
    sha256 = "0qb021c3y2k1ai0vvadv9cd6vacj0lsd5xv2a4ir1hhn0pnc5g59";
  };
  nixpkgsMuslSystemd = builtins.fetchTarball {
    name = "nixos-systemd-musl";
    url = "https://github.com/dtzWill/nixpkgs/archive/" +
      "13510d3dbe08e5bfc5454faf3fe543991dbf6e29.tar.gz";
    sha256 = "090zagbw39fa8fgwvzcmbcr204pxcfrq1rfw7ip5z44naj6gp7qb";
  };
in [ nixpkgs nixpkgsMuslSystemd ]
