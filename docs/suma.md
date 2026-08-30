# Recarga SUMA — recharge flow and iOS feasibility

Reverse-engineered from `apk/recargasuma/com.transermobile.recargasuma.apk`
(RecargaSUMA v1.42.01, versionCode 42, package `com.transermobile.recargasuma`,
TranserMobile / TRANSACCIONES Y SERVICIOS MOBILE SL) with jadx 1.5.6

(`apk/recargasuma/jadx-out/sources`) and apktool 2.12.1.

The app recharges the Valencian interurban transit card **SUMA** (and ATMV
time-based tickets) by **writing the new ticket onto the physical MIFARE card
over NFC** after an online payment. This is the classic offline e-ticketing
model: the card itself stores the balance/ticket, and a recharge is an
**authenticated write to the card**, authorised server-side.

## What the app is

- Native Kotlin/Java (AndroidX + Material), R8-obfuscated (jadx maps the app
  logic to `defpackage/*`; only the 4 activities are named). No native `.so`
  crypto — all sensitive byte tables are Java constants.
- Requires `android.hardware.nfc` (`uses-feature required=true`) and
  `android.permission.NFC` (`AndroidManifest.xml`).
- MainActivity carries an `android.nfc.action.TAG_DISCOVERED` intent-filter,
  and the app also uses `NfcAdapter.enableReaderMode` (`setReaderMode`).
- Payment is **Redsys TPV V-InApp**: embeds the `com.redsys.tpvvinapplibrary`
  SDK (`DirectPaymentActivity` in-app card form + `WebViewPaymentActivity`).
- Crash reporting ACRA (posts to `servlet/InformeAcra`), Firebase (FCM,
  analytics), Google Play SMS Retriever for phone-side OTP.
- TLS is pinned via bundled self-signed CAs `res/raw/ca.crt` / `ca_ok.crt`
  (O=TranSerMobile, CN=transermobile.com).

## Backend

Base: `https://www.transermobile.com/VAExternas/servlet/<Name>` — plain
`application/x-www-form-urlencoded` POST servlets (no OAuth, no JSON). All
endpoint strings + a few public URLs are constants in the config class:

`apk/recargasuma/jadx-out/sources/defpackage/ji.java`:

- `ji.java:27..57` — endpoint constants `p`..`Q`:
  - `Registrar` (Q) — device/phone registration
  - `ObtenerOfertaTitulos_v5` (r), `ObtenerOfertaTitulos_Canje_v5` (s) — ticket offers
  - `ObtenerDatosTPVE_v2` (u) — Redsys TPV (payment) parameters
  - `OC_v4` (C) — recharge order (`numeroPedido`/`fechaPedido`)
  - `ComprobarPagoES` (B) — payment confirmation → returns the card keys + blocks to write
  - `EnviarResultadoLectura` (D), `EnviarResultadoSMS` (E) — telemetry
  - `CheckListas` (M), `CheckLGSB_v4` (L) — card on blacklist / last-good-state
  - `ResetearTitulo` (N), `ObtenerRecargas` (v) — title reset / history
  - `SolicitarEmail` (A), `EnviarIncidencia` (F), `EnviarSMSRetriever` (p),
    `EnviarInfoLista` (q), `EnviarInfoCambioTipoLector` (P), `Subir*` (I/J/K),
    `PubliURL` (H), `ObtenerDatosGen` (t)
  - `suma/comprobarRegistroCU.php` (O) — card-registration check
- `ji.java:16` — app profile `e = "SUMA_01"`.
- `ji.java:14` — card field-layout table `c` (`qu0` records: name, card type,
  block, byte offset, length, encoding). e.g. `TFC`, `TEP` (owner),
  `TT`/`TST`/`TV` (ticket counters), `VBLN`/`VBLB`, `UDLN`, `UFC` (balance
  counter), `TV1FIV`/`TV2FIV` (validity dates), `TF1..TF2` fare structures,
  `MTVSCE`, `MS`. This is the local map used to interpret the card image.
- `ji.java:58..75` — embedded key tables (see *Card write*).

## Authentication / registration

There is **no password login**. The device identifies itself by phone number:

- On first run the app calls `Registrar` with
  `telefono&imei&iccid&codigoVersion&codigo&app=<SUMA_01>&brand&model&vAndroid&nxp=0|1`
  (`TranserActivity.java:418`, `:601`, `:784`, `:967`, `:1150`, `:1333`).
- The phone number is verified with an SMS OTP via Google Play SMS Retriever
  (`EnviarSMSRetriever` / `EnviarResultadoSMS`, `MySMSBroadcastReceiver`);
  `tratarResultadoObtenerTel` stores it and kicks off the app
  (`TranserActivity.java:2608`).
- Later requests also carry `idDispositivo` (`TranserActivity.java:1448`,
  `:1819`).

## Recharge flow

1. **Boot/register** — as above.
2. **Read card (NFC)** — user holds the SUMA card on the phone. The card
   family is chosen by **SAK** (`ht0.java:1153..o()`, logs
   `leerTarjetaExterna: sak:`):
   - `SAK 0x08` → **MIFARE Classic**: `MifareClassic.get(tag)`,
     `authenticateSectorWithKeyA/B`, `readBlock` (`ht0.java:1059..n()`,
     `:137`).
   - `SAK 0x20` → **MIFARE DESFire EV1** via `IsoDep`
     (`ht0.java:1217..p()`, `tratarTarjetaIsoDep`).
   - `SAK 0x30` → **NFC-A / MIFARE Plus** via `NfcA`
     (`ht0.java:1420..q()`, `RATS_PPS_NXP` path).
   - Reads the card image (a ~296-byte buffer `ht0.a`), derives UID, parses
     balance/ticket with the `ji.c` field map, and sanity-checks it
     (`ht0.java:..comprobarTarjeta()`; `TranserActivity.java:1476` re-check).
   - Optionally reports the read to the server (`EnviarResultadoLectura`,
     `EnviarInfoCambioTipoLector`) and consults `CheckListas` /
     `CheckLGSB_v4`.
3. **Pick ticket & pay** — `ObtenerOfertaTitulos_v5` (and `_Canje_v5`) returns
   the fare list (cached locally in the `ofertaTitulos` sqlite table:
   `ofertaID,codTarifa,precio,nViajes,zona,...`). After selecting a title the
   app calls `OC_v4` to create the order (`numeroPedido`/`fechaPedido`) and
   `ObtenerDatosTPVE_v2` to obtain the Redsys parameters
   (`w70.java:134`).
4. **Pay (Redsys)** — `ObtenerDatosTPVE_v2` returns a `dt0` object
   (`dt0.java:5..18`): CodComercio/FUC, Terminal, Moneda, URLOK/URLKO,
   `Simulacion` (test flag), `FirmaSHA256` (Redsys signature, computed
   server-side), Params, Licencia. `e80.java:175..194` feeds this into the
   Redsys SDK (`setEnvironment`, `setLicense`, `setFuc`, `setTerminal`,
   `setCurrency("978")`, `setUrlOK/KO`, `setPaymentMethods`, ...) and launches
   `DirectPaymentActivity` (in-app card entry) or `WebViewPaymentActivity`.
   Billing fields (NIF, name, address, CP) are passed along
   (`e80.java:154`, `DatosFacturacionActivity`).
5. **Confirm payment + receive write material** — `ComprobarPagoES`
   (`TranserActivity.java:2501..tratarResultadoComprobarPago`). Reply bytes:
   - `[0]` = code (`0` not approved, `1` not processed, `2` ok, `3` ok
     (encrypted variant), `5` annulled, …).
   - For ok: `[1..96]` = **`clavesB`** — 96 bytes of per-purchase card keys
     (device-encrypted when variant `3`, `hg1.E(...)` at `hg1.java:262`);
     `[97..]` = **"Datos a grabar"**, comma-separated
     `block|label|hex,` tuples (parsed as `ys0{blockNo, label, hexData}` in
     `TranserActivity.java:2567..2569`, sorted by block no `ys0.java:18`)
     describing exactly which card blocks to overwrite.
   - `hg1.v(...)` installs the keys (`hg1.java:1889..copiarClavesB`): for
     DESFire it sets `ji.X[0..15]` — slot 12 ← key2, slot 13 ← key3, all
     others ← key1 (the "ClaveCarga/C2S1/C2S2"); for Classic it writes 6-byte
     slices into each of the 16 per-sector keys `ji.d0[i][10..15]`.
   - The pending-write record (`grabacionPendiente`, keyed by a hash of the
     card serial + block list) is persisted in SharedPreferences for the
     retry path (`TranserActivity.java:2579..2583`).
6. **Write card (NFC)** — `grabarTarjeta` (`TranserActivity.java:1957..`)
   dispatches by SAK + NXP-chip flag:
   - **DESFire EV1** (`ht0.java:689..h()`):
     - `IsoDep.connect()`, RATS/PPS `E080` to (re)negotiate ISO 14443-4
       (`ht0.java:726..728`).
     - MIFARE **AES mutual authentication** with the per-slot key `ji.X[i5]`:
       first-auth APDU (`70 <block> 40 0100`), decrypt challenge with
       `AES/CBC/NOPADDING` + zero IV, derive session keys K_e/K_m (the
       `RndB/RndA/…/KENC0/KMAC0` dance), then a second auth APDU
       (`ht0.java:731..818`).
     - Each 16-byte block is AES-CBC-encrypted `E(block)` using the session
       key and a per-transaction IV built from the transaction identifier
       `TI`; the write command carries a truncated **AES-CMAC** (`cmdEsc`,
       `CMACcalculado`, `t70.s`), and the card's reply `MACRec2` is verified
       (`ht0.java:819..850`). Final `DeSelect: CA01`.
   - **NFC-A / MIFARE Plus** — same AES structure with the NXP variant
      (`RATS_PPS_NXP`, `Auth1_NXP`, `ht0.java:902..`).
   - **MIFARE Classic** — `authenticateSectorWithKeyA/B` per sector then
      `writeBlock` (`ht0.java:882..894`). Key type is selected via `tipoClave`
      (`TranserActivity.java:1844`), classic cards use the `FF FF FF FF FF FF`
      key or per-sector keys from `ji.d0`.
   - Blocks are written in the order returned by the server; some blocks get a
     checksum/CRC byte appended (`ht0.java:701..714`, `CalcularCRC`).
   - After the write the app verifies the new card state
     (`comprobarRecargaEBloques`, `TranserActivity.java:1476`) and shows
     `tarjeta_grabada_correctamente` (`:1909`).
7. **Pending recharge** — if the write fails the app stores the pending
   record + `numeroPedido/fechaPedido`; on the next open (or on a subsequent
   card presentation) `tratarRecargaPendiente` / `avisoRecargaPendiente`
   (`TranserActivity.java:3824`) re-runs the write using `Comprobar Recarga
   Pendiente` / `OC_v4` data. This is the ~7-minute retry window described in
   the Play listing. `ResetearTitulo` reverts a half-written title.

## Security notes

- **Card keys are a mix of embedded and server-issued material.**
  - Static tables in `ji.java`: `W` (16×16-byte DESFire AES key set),
    `Y` = `FF×6` (Classic fallback key), `Z` and `d0` (16 per-card-profile
    16-byte keys), `X` = 16×16 zero slot table filled at write time.
  - A **real recharge cannot be written without the server**: the DESFire
    session keys (`ClaveCarga`, `ClaveC2S1`, `ClaveC2S2`) and the Classic
    per-sector keys are returned by `ComprobarPagoES` only after a confirmed
    Redsys payment (and the `clavesB` payload is device-encrypted in one of
    the variants).
- The write itself is genuine card-level **AES mutual authentication with
  CMAC verification** (DESFire) or **Crypto1 sector authentication**
  (Classic): the phone acts as a payment-terminal SAM-equivalent. An attacker
  who paid could theoretically reuse the emitted keys within the write window;
  the transaction counters (TI, E/F counters, `contadorE/contadorF`) plus the
  truncated CMAC bound each write to a single order.
- HTTP traffic is plain form-encoded over TLS pinned to the bundled CA. The
  `FirmaSHA256` seen in the payloads is the **Redsys** merchant-signature, not
  an app-level request MAC.

## Can this be replicated on iOS?

The flow splits into a **network/payment half** and an **NFC half**.

### Network + payment half — replicable
`Registrar`, `ObtenerOfertaTitulos_v5`, `OC_v4`, `ObtenerDatosTPVE_v2`,
`ComprobarPagoES`, `ObtenerRecargas` are plain HTTPS servlets; the Redsys step
is a standard web payment (`sis.redsys.es`, `Ds_MerchantParameters`,
`Ds_SignatureVersion`), and the TPV library itself ships an iOS SDK. Nothing
in this half depends on the platform.

### NFC half — the blocker
The app must **read from and write to the physical MIFARE card**. What iOS
Core NFC allows:

- `NFCTagReaderSession` + `NFCMiFareTag` exposes MIFARE via
  `sendMiFareCommand` (native commands) and `sendMiFareISO7816Command`
  (ISO 7816-4 APDUs, for the `.plus` / `.desfire` families). Apple
  documents that `sendMiFareCommand` “doesn’t support the Crypto1 protocol”.
- **MIFARE Classic (SAK 0x08)**: not usable on iOS — Crypto1 is unsupported,
  so key-protected sectors cannot even be read, let alone written. Real-world
  confirmations (e.g. flutter_nfc_kit) list MIFARE Classic block read/write
  as Android-only. This is a hard, permanent blocker for every Classic-era
  SUMA card.
- **MIFARE DESFire EV1 / MIFARE Plus (SAK 0x20 / 0x30)**: the *commands* are
  ordinary ISO 7816-4 wrapped APDUs (select + AES-auth + write with CMAC), so
  `sendMiFareISO7816Command` is, on the API surface, able to carry them — the
  app’s whole `RATS/PPS` + AES-CBC + CMAC sequence maps onto sequence of
  APDUs. No OS-level key protection exists on Android either (the key tables
  are plain Java byte arrays), so a malicious/independent client could port
  the same key handling.
- However: Core NFC sessions are foreground-only and user-initiated;
  DESFire non-NDEF authenticated writes are exactly the area where Apple gives
  no support guarantees; and no production third-party iOS app performs such
  DESFire transit write-backs today. App Review of a CoreNFC app that writes
  another operator’s transit cards using their keys is also a live risk.

### Practical verdict
- **Full replication (the Android app’s core function) on iOS: not feasible.**
  MIFARE Classic cards are unreachable on iOS, and the DESFire write,
  though APDU-reachable in principle, is not something a third-party app can
  ship reliably today, and the scheme is licensed/homologated by the
  Conselleria (the operator controls the server that issues the per-purchase
  keys). Any independent “replica” would amount to using TranserMobile/FGV’s
  credentials and embedded key material against their own cards.
- **Feasible on iOS:** read-only balance/ticket inspection for DESFire cards
  (Core NFC reads work), and—with the operator and Apple—a proper
  **virtual-ticket** route: Express Transit (Wallet) or the EEA “NFC & Secure
  Element” HCE entitlements (iOS 17.4+), neither of which writes to the
  physical card. “Add your SUMA pass to Wallet / phone-as-ticket” is the
  realistic iOS end state, and it is the operator’s decision, not something a
  third party can self-provision.

## Source references (decompiled paths, relative to `apk/recargasuma/jadx-out/sources`)

- `defpackage/ji.java` — config: endpoints (`p`..`Q`), profile `SUMA_01`, card
  field map `c`, embedded key tables `W/X/Y/Z/d0/a0/b0`, protocol bytes
  `E080/E081/CA01/0100`
- `defpackage/ht0.java` — NFC engine: SAK dispatch `o()` `:1153`,
  Classic read `n()` `:1059`, DESFire read `p()` `:1217`, NFC-A read `q()`
  `:1420`, write block `h()` `:689` (DESFire `:718`, Classic `:882`,
  NFCA `:902`), card check `comprobarTarjeta` `:457..686`
- `defpackage/hg1.java` — utils; `copiarClavesB` `v()` `:1889`,
  device-keyed decrypt `E()` `:262`
- `defpackage/dt0.java` — Redsys TPV reply model
- `defpackage/e80.java` — Redsys SDK config + launch `:175..194`
- `com/transermobile/recargasuma/TranserActivity.java` — register params
  `:418`, `grabarTarjeta` `:1957`, block verify `:1476`,
  `tratarResultadoComprobarPago` `:2501`, pending-recharge `:3824`,
  `tratarResultadoObtenerTel` `:2608`
- `com/transermobile/recargasuma/activities/DatosFacturacionActivity.java`,
  `IncidenciaActivity.java` — billing / incident (carries `numeroPedido`,
  `fechaPedido`)