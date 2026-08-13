{
  description = "Gymqrack — live gym-entry QR server (Go)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          gymqrack = pkgs.callPackage ./package.nix { };
        in
        {
          inherit gymqrack;
          default = gymqrack;
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.gymqrack}/bin/gymqrack";
        };
      });

      nixosModules = {
        default = import ./nixos/module.nix;
      };
    };
}
