{ pkgs, lib, config, inputs, ... }:

let
  vivagymAccess = pkgs.callPackage ./package.nix { };
in
{
  name = "vivagym-access";

  # Loads .env into the shell / processes (VIVAGYM_CLIENT_ID, ...)
  dotenv.enable = true;

  languages.go.enable = true;
  languages.go.lsp.enable = true;

  packages = with pkgs; [
    vivagymAccess # nix-built server (bin: vivagym-access)
  ];

  processes.dev.exec = "go run ./cmd/vivagym-access";
  processes.server.exec = "${vivagymAccess}/bin/vivagym-access";

  enterShell = ''
    echo "VivaGym Access dev shell"
    echo "  dev server : devenv up            (go run ./cmd/vivagym-access)"
    echo "  nix build  : devenv up --process server   (vivagym-access)"
  '';
}
