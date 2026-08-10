# VivaGym Wallet

Reverse-engineered API client and live gym-entry QR server for the VivaGym
Group app (`com.myvitale.vivagym.group`).

## What this does

- `cmd/` + `internal/` — a **Go** (stdlib `net/http`) **stateless proxy** to the
  VivaGym API. Members log in through the web UI with their own VivaGym
  credentials and get a **live, auto-refreshing gym-entry QR code**. The server
  never stores credentials; each user's token pair lives in an HttpOnly cookie
  owned by their browser.
- `nixos/` — NixOS module to run the server as a hardened systemd service.
- `cmd/make-pass` — generates an (unsigned) Apple Wallet `.pkpass` launcher for
  the QR page.
- `docs/vivagym-api.md` — notes on the API endpoints and auth flow (reverse
  engineered from the APK in `apk/`).

## How auth works

1. User submits their VivaGym email + password on the web page.
2. The server proxies the OAuth flow (client_credentials → `exerp/newAuth`),
   gets the member's access + refresh tokens, and hands them to the browser in
   an **HttpOnly + Secure + SameSite=Lax cookie** — the server keeps nothing.
3. On each `/qr` request the server reads the token pair from the cookie,
   refreshing the ~10-minute access token transparently when needed, and
   forwards the bearer token to `api/v2.0/exerp/qr`.

Tokens are never stored in `localStorage`; do not move them there.

## Development (devenv)

```sh
devenv up          # run the dev server (go run) on port 4567
devenv up --process make-pass  # build the wallet pass (needs Apple signing certs)
```

Env: copy `.env.example` to `.env`. Members sign in through the web UI — no
credentials are stored in `.env`.

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
    trustProxy = true;   # set behind a reverse proxy (rate limiting by real IP)
  };
}
```

## Layout

```
├── apk/            # original .xapk (untracked binary)
├── cmd/            # vivagym-wallet (server) + make-pass (.pkpass generator)
├── docs/           # API reverse-engineering notes
├── internal/       # server (handlers, cookie, rate limiting) + VivaGym API client + QR
├── nixos/          # NixOS service module
├── public/         # live-QR web page
├── devenv.nix      # developer environment
├── flake.nix       # nix package + module outputs
└── package.nix     # buildGoModule derivation
```

## Tests

```sh
go test ./...
```
