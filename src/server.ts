import { serve } from "@hono/node-server";
import { serveStatic } from "@hono/node-server/serve-static";
import { getConnInfo } from "@hono/node-server/conninfo";
import { deleteCookie, getCookie, setCookie } from "hono/cookie";
import { Hono } from "hono";
import type { Context } from "hono";
import type { ContentfulStatusCode } from "hono/utils/http-status";
import QRCode from "qrcode";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { login, refreshTokens, fetchQr, VivaGymError } from "./vivagym.js";

const PORT = Number(process.env.PORT) || 4567;
const HOST = process.env.HOST || "0.0.0.0";
const PUBLIC_URL = (process.env.PUBLIC_URL || `http://localhost:${PORT}`).replace(/\/$/, "");
const PUBLIC_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "public");
const LOCALE = process.env.VIVAGYM_LOCALE || "es";

const COOKIE_NAME = "vivagym_tokens";
const COOKIE_SECURE = PUBLIC_URL.startsWith("https://");
const COOKIE_MAX_AGE = (Number(process.env.COOKIE_MAX_AGE_DAYS) || 7) * 86_400;
const LOGIN_RATE_PER_MIN = Number(process.env.LOGIN_RATE_PER_MIN) || 10;

// Client-owned VivaGym token pair, stored in an HttpOnly cookie.
// The server never persists it anywhere.
interface Tokens {
  accessToken: string;
  refreshToken: string;
  expiresIn: number;
  issuedAt: number;
  email: string;
}

function encodeTokens(t: Tokens): string {
  return Buffer.from(JSON.stringify(t)).toString("base64url");
}

function decodeTokens(raw: string): Tokens | null {
  try {
    const t = JSON.parse(Buffer.from(raw, "base64url").toString("utf8")) as Tokens;
    return t.accessToken ? t : null;
  } catch {
    return null;
  }
}

function readTokens(c: Context): Tokens | null {
  const raw = getCookie(c, COOKIE_NAME);
  return raw ? decodeTokens(raw) : null;
}

function writeTokens(c: Context, t: Tokens): void {
  setCookie(c, COOKIE_NAME, encodeTokens(t), {
    httpOnly: true,
    secure: COOKIE_SECURE,
    sameSite: "Lax",
    path: "/",
    maxAge: COOKIE_MAX_AGE,
  });
}

function clearTokens(c: Context): void {
  deleteCookie(c, COOKIE_NAME, { path: "/" });
}

function isAccessTokenValid(t: Tokens): boolean {
  return Boolean(t.accessToken) && Date.now() < t.issuedAt + t.expiresIn * 1000 - 10_000;
}

// Return a usable access token, refreshing (and rotating the cookie) if needed.
async function validTokens(c: Context): Promise<Tokens | null> {
  let t = readTokens(c);
  if (!t) return null;
  if (isAccessTokenValid(t)) return t;
  if (!t.refreshToken) return null;
  try {
    const fresh = await refreshTokens(t.refreshToken);
    t = { ...t, ...fresh, issuedAt: Date.now() };
    writeTokens(c, t);
    return t;
  } catch {
    clearTokens(c);
    return null;
  }
}

// Fetch the QR payload, transparently refreshing once if VivaGym rejects the token.
async function qrResult(
  c: Context,
): Promise<{ payload: string } | { error: string; status: ContentfulStatusCode }> {
  let t = await validTokens(c);
  if (!t) return { error: "Not authenticated", status: 401 };

  for (let attempt = 0; ; attempt++) {
    try {
      return { payload: await fetchQr(t.accessToken) };
    } catch (err) {
      const vg = err instanceof VivaGymError ? err : null;
      if (vg?.status === 401 && attempt === 0 && t.refreshToken) {
        try {
          const fresh = await refreshTokens(t.refreshToken);
          t = { ...t, ...fresh, issuedAt: Date.now() };
          writeTokens(c, t);
          continue;
        } catch {
          // fall through: token is dead, force re-login
        }
      }
      if (vg?.status === 401) {
        clearTokens(c);
        return { error: "Session expired, log in again", status: 401 };
      }
      return { error: err instanceof Error ? err.message : String(err), status: 502 };
    }
  }
}

// Login rate limiting (simple in-memory fixed window)
const attempts = new Map<string, { count: number; resetAt: number }>();

function clientIp(c: Context): string {
  const forwarded = c.req.header("x-forwarded-for");
  if (process.env.TRUST_PROXY === "1" && forwarded) return forwarded.split(",")[0].trim();
  try {
    return getConnInfo(c).remote.address || "unknown";
  } catch {
    return "unknown";
  }
}

function allowLogin(ip: string): boolean {
  const now = Date.now();
  const entry = attempts.get(ip);
  if (!entry || entry.resetAt < now) {
    attempts.set(ip, { count: 1, resetAt: now + 60_000 });
    return true;
  }
  entry.count += 1;
  return entry.count <= LOGIN_RATE_PER_MIN;
}

const app = new Hono();

app.get("/health", (c) => c.json({ ok: true }));

app.get("/auth/session", (c) => {
  const t = readTokens(c);
  if (!t) return c.json({ authenticated: false }, 401);
  return c.json({ authenticated: true, email: t.email });
});

app.post("/auth/login", async (c) => {
  if (!allowLogin(clientIp(c))) {
    return c.json({ error: "Too many attempts, try again later" }, 429);
  }

  let body: { email?: unknown; password?: unknown };
  try {
    body = await c.req.json();
  } catch {
    return c.json({ error: "Invalid request" }, 400);
  }
  const email = typeof body.email === "string" ? body.email.trim().toLowerCase() : "";
  const password = typeof body.password === "string" ? body.password : "";
  if (!email || !password || email.length > 254 || password.length > 256 || !email.includes("@")) {
    return c.json({ error: "Invalid email or password" }, 400);
  }

  try {
    const pair = await login(email, password, LOCALE);
    writeTokens(c, { ...pair, issuedAt: Date.now(), email });
    attempts.delete(clientIp(c));
    return c.json({ email });
  } catch (err) {
    console.warn("login failed for", email, "-", err instanceof Error ? err.message : err);
    return c.json({ error: "Invalid credentials" }, 401);
  }
});

app.post("/auth/logout", (c) => {
  clearTokens(c);
  return c.json({ ok: true });
});

app.get("/qr", async (c) => {
  const result = await qrResult(c);
  if ("payload" in result) {
    return c.json({ payload: result.payload, issuedAt: new Date().toISOString() });
  }
  return c.json({ error: result.error }, result.status);
});

app.get("/qr.png", async (c) => {
  const result = await qrResult(c);
  if ("error" in result) return c.text(result.error, result.status);
  try {
    const buffer = await QRCode.toBuffer(result.payload, { width: 512, margin: 2 });
    return c.body(Uint8Array.from(buffer), 200, { "Content-Type": "image/png" });
  } catch (err) {
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
