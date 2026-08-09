{
  description = "VivaGym Wallet — live gym-entry QR server (Hono + TypeScript)";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (system:
        let
          pkgs = nixpkgs.legacyPackages.${system};
          vivagymWallet = pkgs.callPackage ./package.nix { };
        in
        {
          "vivagym-wallet" = vivagymWallet;
          default = vivagymWallet;
        });

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.vivagym-wallet}/bin/vivagym-wallet";
        };
      });

      nixosModules = {
        default = import ./nixos/module.nix;
      };
    };
}
