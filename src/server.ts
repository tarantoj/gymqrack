import { serve } from "@hono/node-server";
import { serveStatic } from "@hono/node-server/serve-static";
import { Hono } from "hono";
import QRCode from "qrcode";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { VivaGymClient } from "./vivagym.js";

const PORT = Number(process.env.PORT) || 4567;
const HOST = process.env.HOST || "0.0.0.0";
const PUBLIC_URL = (process.env.PUBLIC_URL || `http://localhost:${PORT}`).replace(/\/$/, "");
const PUBLIC_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "public");

const email = process.env.VIVAGYM_EMAIL;
const password = process.env.VIVAGYM_PASSWORD;
if (!email || !password) {
  console.error("Missing VIVAGYM_EMAIL / VIVAGYM_PASSWORD (see .env / devenv dotenv)");
  process.exit(1);
}

const client = new VivaGymClient({
  email,
  password,
  locale: process.env.VIVAGYM_LOCALE || "es",
});

const app = new Hono();

app.get("/health", (c) => c.json({ ok: true }));

// Raw QR payload (the exerp:checkin:... string)
app.get("/qr", async (c) => {
  try {
    const payload = await client.getQrPayload();
    return c.json({ payload, issuedAt: new Date().toISOString() });
  } catch (err) {
    console.error("GET /qr", err);
    return c.json({ error: err instanceof Error ? err.message : String(err) }, 502);
  }
});

// QR as PNG — a fresh token is minted on every request
app.get("/qr.png", async (c) => {
  try {
    const payload = await client.getQrPayload();
    const buffer = await QRCode.toBuffer(payload, { width: 512, margin: 2 });
    return c.body(Uint8Array.from(buffer), 200, { "Content-Type": "image/png" });
  } catch (err) {
    console.error("GET /qr.png", err);
    return c.text(err instanceof Error ? err.message : String(err), 502);
  }
});

app.use("/*", serveStatic({ root: PUBLIC_DIR }));

serve({ fetch: app.fetch, port: PORT, hostname: HOST }, () => {
  console.log(`VivaGym live QR running: ${PUBLIC_URL}`);
  console.log(`  live screen : ${PUBLIC_URL}/`);
  console.log(`  QR PNG      : ${PUBLIC_URL}/qr.png`);
  console.log(`  JSON payload: ${PUBLIC_URL}/qr`);
});
