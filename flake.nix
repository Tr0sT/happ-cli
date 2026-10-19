{
  description = "Headless Happ subscription client for Linux and NixOS";
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      eachSystem = nixpkgs.lib.genAttrs systems;
    in {
      packages = eachSystem (system:
        let pkgs = import nixpkgs { inherit system; };
        in rec {
          happ-cli = pkgs.callPackage ./nix/package.nix { };
          default = happ-cli;
        });
      nixosModules.default = import ./nix/module.nix;
      devShells = eachSystem (system:
        let pkgs = import nixpkgs { inherit system; };
        in { default = pkgs.mkShell { packages = [ pkgs.go_1_26 ]; }; });
    };
}
