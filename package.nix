# Builds the VivaGym wallet server (Go) into a runnable nix package. The
# resulting derivation installs `vivagym-wallet` and `make-pass` binaries and
# the static web UI. Runtime config comes from environment variables
# (VIVAGYM_CLIENT_ID, VIVAGYM_CLIENT_SECRET, VIVAGYM_LOCALE, PORT, PUBLIC_URL,
# COOKIE_MAX_AGE_DAYS, LOGIN_RATE_PER_MIN, TRUST_PROXY).

{ lib, buildGoModule, openssl, makeWrapper }:

buildGoModule {
  pname = "vivagym-wallet";
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

  vendorHash = "sha256-TCZzdWhLh94hLAeyfcnHHnKuN1TlIznWq6P8XEghBTE=";

  nativeBuildInputs = [ makeWrapper ];

  # openssl is only used at runtime by make-pass (PKCS#7 pass signing).
  buildInputs = [ openssl ];

  postInstall = ''
    mkdir -p $out/share/vivagym-wallet
    cp -r public $out/share/vivagym-wallet/public
    wrapProgram $out/bin/make-pass \
      --prefix PATH : ${openssl}/bin
  '';

  meta = {
    description = "VivaGym gym-entry live QR server (Wallet launcher)";
    homepage = "https://github.com/";
    license = lib.licenses.mit;
    mainProgram = "vivagym-wallet";
  };
}
