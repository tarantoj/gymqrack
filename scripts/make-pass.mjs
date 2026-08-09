// Builds a .pkpass for the VivaGym entry QR.
//
// The pass's barcode encodes the URL of the live QR screen (Option C:
// Wallet acts as a launcher — iOS does not auto-open URLs from passes, so the
// turnstile scans the QR shown in the webview/page, not this static barcode).
//
// Signing requires an Apple "Pass Type ID" certificate. Provide PEM material
// via env, otherwise the script emits an unsigned zip (not installable):
//   APPLE_TEAM_ID, PASS_TYPE_IDENTIFIER, SIGNING_CERT_PEM, SIGNING_KEY_PEM, WWDR_PEM

import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import zlib from "node:zlib";
import { execSync } from "node:child_process";

const ROOT = path.resolve(import.meta.dirname, "..");
const PUBLIC_URL = (process.env.PUBLIC_URL || "http://localhost:3000").replace(/\/$/, "");
const PASS_TYPE_ID = process.env.PASS_TYPE_IDENTIFIER || "pass.com.vivagym.entry";
const TEAM_ID = process.env.APPLE_TEAM_ID || "YOUR_TEAM_ID";
const OUT = process.env.OUT_PASS || path.join(ROOT, "dist", "vivagym.pkpass");

// ---- minimal PNG writer (solid colour placeholder icons) -----------------

const CRC_TABLE = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    t[n] = c >>> 0;
  }
  return t;
})();

function crc32(buf) {
  let c = 0xffffffff;
  for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
  return (c ^ 0xffffffff) >>> 0;
}

function pngChunk(type, data) {
  const len = Buffer.alloc(4);
  len.writeUInt32BE(data.length);
  const typeBuf = Buffer.from(type, "ascii");
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(Buffer.concat([typeBuf, data])));
  return Buffer.concat([len, typeBuf, data, crc]);
}

function solidPng(size, [r, g, b]) {
  const stride = size * 3;
  const raw = Buffer.alloc((stride + 1) * size);
  for (let y = 0; y < size; y++) {
    raw[y * (stride + 1)] = 0;
    for (let x = 0; x < size; x++) {
      const i = y * (stride + 1) + 1 + x * 3;
      raw[i] = r;
      raw[i + 1] = g;
      raw[i + 2] = b;
    }
  }
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(size, 0);
  ihdr.writeUInt32BE(size, 4);
  ihdr[8] = 8; // bit depth
  ihdr[9] = 2; // truecolour RGB
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    pngChunk("IHDR", ihdr),
    pngChunk("IDAT", zlib.deflateSync(raw)),
    pngChunk("IEND", Buffer.alloc(0)),
  ]);
}

// ---- pass content --------------------------------------------------------

const serial = crypto.randomUUID();
const qrUrl = `${PUBLIC_URL}/qr`;

const passJson = {
  formatVersion: 1,
  passTypeIdentifier: PASS_TYPE_ID,
  teamIdentifier: TEAM_ID,
  serialNumber: serial,
  organizationName: "VivaGym",
  description: "VivaGym gym entry QR",
  logoText: "VivaGym",
  foregroundColor: "rgb(255,255,255)",
  backgroundColor: "rgb(253,80,0)",
  labelColor: "rgb(0,0,0)",
  barcode: {
    format: "PKBarcodeFormatQR",
    message: qrUrl,
    messageEncoding: "utf-8",
  },
  storeCard: {
    primaryFields: [
      { key: "entry", label: "Entrada / Access", value: "Abrir QR / Open QR" },
    ],
    auxiliaryFields: [
      { key: "refresh", label: "Actualización", value: "Se renueva cada minuto" },
    ],
    backFields: [
      { key: "url", label: "QR URL", value: qrUrl },
      {
        key: "usage",
        label: "Uso / Usage",
        value:
          "iOS no abre URLs automáticamente desde el pase. Abre la URL (o esta web) y muestra el QR en pantalla para el torno. / iOS does not open URLs from a pass automatically. Open the URL and show the QR on screen at the turnstile.",
      },
    ],
  },
};

// ---- build + zip ---------------------------------------------------------

const buildDir = fs.mkdtempSync(path.join(os.tmpdir(), "vivagym-pass-"));
const files = new Map();

files.set("pass.json", JSON.stringify(passJson, null, 2));
files.set("icon.png", solidPng(29, [253, 80, 0]));
files.set("icon@2x.png", solidPng(58, [253, 80, 0]));
files.set("logo.png", solidPng(58, [253, 80, 0]));
files.set("logo@2x.png", solidPng(116, [253, 80, 0]));

const manifest = {};
for (const [name, content] of files) {
  const buf = Buffer.isBuffer(content) ? content : Buffer.from(content);
  manifest[name] = crypto.createHash("sha1").update(buf).digest("hex");
}
files.set("manifest.json", JSON.stringify(manifest));

const certPem = process.env.SIGNING_CERT_PEM;
const keyPem = process.env.SIGNING_KEY_PEM;
const wwdrPem = process.env.WWDR_PEM;

if (certPem && keyPem) {
  fs.writeFileSync(path.join(buildDir, "cert.pem"), certPem);
  fs.writeFileSync(path.join(buildDir, "key.pem"), keyPem);
  if (wwdrPem) fs.writeFileSync(path.join(buildDir, "wwdr.pem"), wwdrPem);

  const args = [
    "smime", "-sign", "-binary",
    "-in", "manifest.json",
    "-out", "signature",
    "-signer", "cert.pem",
    "-inkey", "key.pem",
    "-outform", "DER",
    "-noattr",
  ];
  if (wwdrPem) args.push("-certfile", "wwdr.pem");
  execSync(`openssl ${args.join(" ")}`, { cwd: buildDir, stdio: "inherit" });
  files.set("signature", fs.readFileSync(path.join(buildDir, "signature")));
  console.log("Signed signature with provided certificate.");
} else {
  console.warn(
    "WARNING: no signing cert (SIGNING_CERT_PEM/SIGNING_KEY_PEM/WWDR_PEM). " +
      "The .pkpass will be UNSIGNED and cannot be installed in Wallet.",
  );
}

for (const [name, content] of files) {
  fs.writeFileSync(path.join(buildDir, name), content);
}

fs.mkdirSync(path.dirname(OUT), { recursive: true });
execSync(`zip -r -q ${JSON.stringify(OUT)} .`, { cwd: buildDir, stdio: "inherit" });
fs.rmSync(buildDir, { recursive: true, force: true });

console.log(`Pass written to ${OUT}`);
console.log(`Barcode message (QR URL): ${qrUrl}`);
