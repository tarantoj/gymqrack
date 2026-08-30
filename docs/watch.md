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
│   ├── QRImageRenderer.swift    # pure-Swift encoder (Nayuki) + CoreGraphics
│   ├── QRCodeGenerator/         # vendored Nayuki QR library (MIT, Foundation-only)
│   ├── KeychainSessionStore.swift  # per-app keychain session (no shared group)
│   └── SessionSync.swift        # WatchConnectivity session transfer + nudges
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

Set your signing team once: `DEVELOPMENT_TEAM` in `project.yml`, then run on
your paired Apple Watch. Bundle IDs are
`com.vivagym.VivaGymCompanion` / `com.vivagym.VivaGymCompanion.watchkitapp`
(watchOS requires the watch bundle id to be prefixed by its companion's).

## How it works

- **Login (iPhone).** The companion runs the two-stage OAuth and stores the
  session (`access/refresh` tokens, issue/expiry) in its own Keychain. Only
  tokens are stored — never the password.
- **Session (watch).** Each app keeps its own Keychain copy; the companion
  pushes the session to the watch over WatchConnectivity (`SessionSync`) right
  after login/sign-out, and a watch that has nothing stored requests it back
  from a reachable phone at launch. Cross-app keychain access groups were
  dropped: they are unreliable for standalone watch apps on free teams. If the
  access token is within 10 s of expiry the watch refreshes via
  `api/email/refresh`, mirroring the gymqrack safety margin.
- **Proximity.** On launch the watch asks for notification permission and
  streams location with `CLLocationUpdate.liveUpdates()` (the watchOS 26 SDK
  removed `requestLocation()`, `CLCircularRegion` monitoring and the
  authorization-request calls). It fetches `user-clubs` and, from each fix,
  within 250 m of the nearest club shows the QR; otherwise a distance view does
  ("Show QR anyway" forces it). Moving inside the 200 m geofence radius is
  detected in-code and posts an "open to scan" notification once per visit.
- **QR.** `api/v2.0/exerp/qr` returns an **opaque** payload
  (`exerp:checkin:<memberRef>-<epochMillis>-<digest>`, server-signed, expires
  server-side). It renders unchanged with a vendored pure-Swift QR encoder
  (Nayuki, MIT) at error-correction level L plus a 4-module quiet zone — the
  same parameters as the VivaGym app's ZXing encoder and gymqrack's
  `rsc.io/qr`, so turnstile scanners read it identically (other encoders'
  `CIQRCodeGenerator` is unavailable on watchOS). It re-fetches every 45 s while
  visible and has a "New code" button for an immediate refresh before scanning.

## Caveats

- The watchOS 26 SDK **removed background region geofencing** (both
  `locationManager:didEnterRegion:` and `CLMonitor` are unavailable on
  watchOS), so the "open to scan" arrival notification now fires from the live
  location stream while the app has runtime — it no longer wakes a terminated
  watch app. For personal sideloading via Xcode it works as-is; the 250 m gate
  on app-open + "Show QR anyway" works regardless.
- The QR expiry window is unknown (enforced inside the Exerp digest); 45 s
  refresh is deliberately conservative.
- Free (personal-team) provisioning signs for 7 days; re-install after expiry.
- Build tooling is Xcode/XcodeGen; `xcodegen generate` and the unit tests run
  in the devenv shell, and `build`/`build-ios`/`test` need full Xcode (with
  `/Applications/Xcode.app` present, the shell routes `DEVELOPER_DIR`/`LD` to
  it).