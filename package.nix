# Builds the VivaGym wallet server (Hono + TypeScript) into a runnable nix
# package. The resulting derivation installs a `vivagym-wallet` binary that
# serves the live QR screen; runtime config comes from environment variables
# (VIVAGYM_EMAIL, VIVAGYM_PASSWORD, VIVAGYM_LOCALE, PORT, PUBLIC_URL).
#
# Usage: import ./package.nix (or callPackage), e.g. in devenv.nix:
#   let package = import ./package.nix { inherit pkgs; }; in ...

{ lib, buildNpmPackage, nodejs_24 }:

buildNpmPackage {
  pname = "vivagym-wallet";
  version = "0.2.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./package.json
      ./package-lock.json
      ./tsconfig.json
      ./src
      ./public
    ];
  };

  nodejs = nodejs_24;

  npmDepsHash = "sha256-cgDLaHMxisZa/AehkZn/Bn8NiqbCXaAtn7YYTeNw2PU=";

  # `npm run build` (tsc -> dist/)
  npmBuildScript = "build";

  installPhase = ''
    runHook preInstall
    mkdir -p $out/libexec $out/bin
    cp package.json $out/libexec/
    cp -r dist $out/libexec/dist
    cp -r public $out/libexec/public
    cp -r node_modules $out/libexec/node_modules
    cat > $out/bin/vivagym-wallet <<EOF
    #!/bin/sh
    exec ${nodejs_24}/bin/node $out/libexec/dist/server.js
    EOF
    chmod +x $out/bin/vivagym-wallet
    runHook postInstall
  '';

  meta = {
    description = "VivaGym gym-entry live QR server (Wallet launcher)";
    homepage = "https://github.com/";
    license = lib.licenses.mit;
    mainProgram = "vivagym-wallet";
  };
}
