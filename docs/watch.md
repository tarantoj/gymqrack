# Apple Watch app (apple/)

A watchOS app that shows a live VivaGym entry QR when you're near your club,
with an iPhone companion app for the (keyboard-hostile) login.

It talks **directly to VivaGym** (`vivagym.myvitale.com`) — the gymqrack server
is not involved. It reuses the same reverse-engineered flow documented in
[`api.md`](api.md): `client_credentials` grant → `exerp/newAuth` → bearer token,
with `api/email/refresh` for rotation and `api/v2.0/exerp/qr` for the entry QR.
Club coordinates come from `api/v2.0/exerp/user-clubs` (`Center` fields).

## Layout

```
apple/
├── project.yml              # XcodeGen spec (VivaGym.xcodeproj is generated)
├── devenv.nix|yaml|lock     # standalone devenv env (composed into the root)
├── VivaGymKit/              # shared sources compiled into both apps + tests
│   ├── VivaGymConfig.swift      # client id/secret, locale, thresholds
│   ├── VivaGymClient.swift      # URLSession client mirroring internal/vivagym
│   ├── Models.swift             # Session, Center
│   ├── KeychainSessionStore.swift  # shared-access-group keychain session
│   ├── QRImageRenderer.swift    # CoreImage, EC level L, 4-module quiet zone
│   └── SessionSync.swift        # WatchConnectivity nudge on session change
├── VivaGymWatch/            # watchOS 10+ app (QR, proximity gate, geofence)
├── VivaGymWatchCompanion/   # iOS 17+ app (login only)
└── VivaGymKitTests/         # unit tests (no host app, mock URL protocol)
```

## Build / run

Prerequisite: full Xcode with the watchOS SDK (`xcodebuild` is *not* available
from Command Line Tools alone).

```sh
cd apple && devenv shell       # standalone Apple env (xcodegen on PATH)
devenv shell generate         # xcodegen generate
devenv shell build            # build VivaGymWatch (watchOS)
devenv shell build-ios        # build VivaGymWatchCompanion (iOS Simulator)
devenv shell test             # run VivaGymKitTests
```

Set your signing team once: `DEVELOPMENT_TEAM` in `project.yml` (or in Xcode)
for both targets, then run on your paired Apple Watch. Bundle IDs are
`com.vivagym.VivaGymWatch` / `com.vivagym.VivaGymCompanion`.

## How it works

- **Login (iPhone).** The companion runs the two-stage OAuth and stores the
  session (`access/refresh` tokens, issue/expiry) in a Keychain access group
  (`$(AppIdentifierPrefix)vivagym.session`) shared with the watch app. Only
  tokens are stored — never the password.
- **Session (watch).** The watch reads the shared keychain; if the access token
  is within 10 s of expiry it refreshes via `api/email/refresh`, mirroring the
  gymqrack safety margin. A WatchConnectivity message makes an already-open
  watch app reload right after login/sign-out.
- **Proximity.** On launch the watch asks for Always location, requests a
  one-shot fix, fetches `user-clubs`, and registers a 200 m `CLCircularRegion`
  per club. Within 250 m of the nearest club the QR shows; otherwise a distance
  view does ("Show QR anyway" forces it). Entering a geofence posts an
  "open to scan" notification that launches the app into a fresh QR.
- **QR.** `api/v2.0/exerp/qr` returns an **opaque** payload
  (`exerp:checkin:<memberRef>-<epochMillis>-<digest>`, server-signed, expires
  server-side). The app renders it unchanged with `CIQRCodeGenerator` at
  error-correction level L plus a 4-module quiet zone — the same parameters as
  the VivaGym app's ZXing encoder and gymqrack's `rsc.io/qr`, so turnstile
  scanners read it identically. It re-fetches every 45 s while visible and has
  a "New code" button for an immediate refresh before scanning.

## Caveats

- WatchOS background geofence delivery (the arrival notification) can be
  finicky and, for App Store release, needs Apple's background-location
  approval; for personal sideloading via Xcode it works as-is. The 250 m gate
  on app-open + "Show QR anyway" works regardless.
- The QR expiry window is unknown (enforced inside the Exerp digest); 45 s
  refresh is deliberately conservative.
- Build tooling is Xcode/XcodeGen; only `xcodegen generate` and the unit tests
  run in the devenv shell (no Xcode required).