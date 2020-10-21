{ lib, rustPlatform, fetchFromGitHub }:

rustPlatform.buildRustPackage rec {
  name = "example";
  src = ../rust;
  cargoSha256 = "1rgnvk761434pz66317a1a5yr639d8j5nybpbf6459dzqkp3m417";
}
