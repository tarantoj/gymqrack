# VivaGym Wallet

Reverse-engineered API client and live gym-entry QR server for the VivaGym
Group app (`com.myvitale.vivagym.group`).

## What this does

- `src/` — Hono + TypeScript server that authenticates against the VivaGym API
  and serves a **live, auto-refreshing gym-entry QR code** on a web page.
- `nixos/` — NixOS module to run the server as a hardened systemd service.
- `scripts/make-pass.mjs` — generates an (unsigned) Apple Wallet `.pkpass`
  launcher for the QR page.
- `docs/vivagym-api.md` — notes on the API endpoints and auth flow (reverse
  engineered from the APK in `apk/`).

## Development (devenv)

```sh
devenv up          # run the dev server (tsx watch) on port 4567
npm run make-pass  # build the wallet pass (needs Apple signing certs)
```

Env: copy `.env.example` to `.env` and set `VIVAGYM_EMAIL` / `VIVAGYM_PASSWORD`.

## Nix

```sh
nix build .#vivagym-wallet       # build the server package
nix run .#                       # run it (env vars from the environment)
```

NixOS module:

```nix
{
  inputs.vivagym-wallet.url = "github:you/vivagym";
  ...
  services.vivagym-wallet = {
    enable = true;
    publicUrl = "https://qr.example.com";
    email = "member@example.com";
    passwordFile = config.age.secrets."vivagym-password".path;
  };
}
```

## Layout

```
├── apk/            # original .xapk (untracked binary)
├── docs/           # API reverse-engineering notes
├── nixos/          # NixOS service module
├── public/         # live-QR web page
├── scripts/        # .pkpass generator
├── src/            # Hono server + VivaGym API client
├── devenv.nix      # developer environment
├── flake.nix       # nix package + module outputs
└── package.nix     # buildNpmPackage derivation
```
