# Builds the Gymqrack live-QR server (Go) into a runnable nix package. The
# resulting derivation installs the `gymqrack` binary and the static web UI.
# Runtime config comes from environment variables
# (GYMQRACK_CLIENT_ID, GYMQRACK_CLIENT_SECRET, GYMQRACK_LOCALE, PORT,
# PUBLIC_URL, COOKIE_MAX_AGE_DAYS, LOGIN_RATE_PER_MIN, TRUST_PROXY).

{ lib, buildGoModule }:

buildGoModule {
  pname = "gymqrack";
  version = "0.2.0";

  src = lib.fileset.toSource {
    root = ./.;
    fileset = lib.fileset.unions [
      ./go.mod
      ./go.sum
      ./cmd
      ./internal
      ./public
    ];
  };

  vendorHash = "sha256-j3eeyLBQ1fBpnQtf6+pyXSQ9mdUKSj5soQd6JT47PpM=";

  postInstall = ''
    mkdir -p $out/share/gymqrack
    cp -r public $out/share/gymqrack/public
  '';

  meta = {
    description = "Gymqrack — gym-entry live QR server";
    homepage = "https://github.com/";
    license = lib.licenses.mit;
    mainProgram = "gymqrack";
  };
}
