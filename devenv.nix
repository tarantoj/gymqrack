{ pkgs, lib, config, inputs, ... }:

let
  vivagymWallet = pkgs.callPackage ./package.nix { };
in
{
  name = "vivagym-wallet";

  # Loads .env into the shell / processes (VIVAGYM_CLIENT_ID, ...)
  dotenv.enable = true;

  languages.go.enable = true;
  languages.go.lsp.enable = true;

  packages = with pkgs; [
    openssl # PKCS#7 pass signature (cmd/make-pass)
    zip     # .pkpass packaging
    vivagymWallet # nix-built server (bin: vivagym-wallet)
  ];

  processes.dev.exec = "go run ./cmd/vivagym-wallet";
  processes.server.exec = "${vivagymWallet}/bin/vivagym-wallet";
  processes.make-pass.exec = "${vivagymWallet}/bin/make-pass";

  enterShell = ''
    echo "VivaGym Wallet dev shell"
    echo "  dev server : devenv up            (go run ./cmd/vivagym-wallet)"
    echo "  nix build  : devenv up --process server   (vivagym-wallet)"
    echo "  wallet pass: devenv up --process make-pass  (requires Apple signing certs, see cmd/make-pass)"
  '';
}
