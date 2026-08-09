{ pkgs, lib, config, inputs, ... }:

let
  vivagymWallet = pkgs.callPackage ./package.nix { };
in
{
  name = "vivagym-wallet";

  # Loads .env into the shell / processes (VIVAGYM_EMAIL, VIVAGYM_PASSWORD, ...)
  dotenv.enable = true;

  languages.javascript.enable = true;
  languages.javascript.package = pkgs.nodejs_24;
  languages.javascript.npm.enable = true;

  packages = with pkgs; [
    openssl # PKCS#7 pass signature (scripts/make-pass.mjs)
    zip     # .pkpass packaging
    vivagymWallet # nix-built server (bin: vivagym-wallet)
  ];

  processes.dev.exec = "npm run dev";
  processes.server.exec = "${vivagymWallet}/bin/vivagym-wallet";

  enterShell = ''
    if [ ! -d node_modules ]; then
      echo "Installing npm dependencies…"
      npm install
    fi
    echo "VivaGym Wallet dev shell"
    echo "  dev server : devenv up            (npm run dev)"
    echo "  nix build  : devenv up --process server   (vivagym-wallet)"
    echo "  wallet pass: npm run make-pass    (requires Apple signing certs, see scripts/make-pass.mjs)"
  '';
}
