// Stateless proxy client for the VivaGym API.
//
// The server holds no user state: every request is an independent OAuth2
// call, and VivaGym tokens are owned by the browser (see src/server.ts).

const BASE_URL = "https://vivagym.myvitale.com";

// Hardcoded in the app (com.vitale.coredata.BuildConfig)
const CLIENT_ID = "4_43uq8rgou3y88ckkk0sgg8c408w4gwsssg8owg0ow4wcocgw0w";
const CLIENT_SECRET = "1uiljdab2misc4owsc0kg0cw0kgw0k0gkgk0k8k488w8sskk4s";

const APP_NAME = "vivagym";

export interface TokenPair {
  accessToken: string;
  refreshToken: string;
  /** access token lifetime in seconds */
  expiresIn: number;
}

export class VivaGymError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

async function request(url: string, init?: RequestInit): Promise<unknown> {
  const res = await fetch(url, init);
  const text = await res.text();
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch {
    data = text;
  }
  if (!res.ok) {
    const message =
      (data as { message?: string })?.message ??
      (data as { error_description?: string })?.error_description ??
      `VivaGym API ${res.status}`;
    throw new VivaGymError(message, res.status);
  }
  return data;
}

interface TokenResponse {
  access_token?: string;
  refresh_token?: string;
  expires_in?: number;
}

// Stage 1: anonymous app-level token (client_credentials grant)
async function clientCredentials(): Promise<string> {
  const data = (await request(`${BASE_URL}/oauth/v2/token`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      grant_type: "client_credentials",
      client_id: CLIENT_ID,
      client_secret: CLIENT_SECRET,
    }),
  })) as TokenResponse;
  if (!data.access_token) throw new VivaGymError("client_credentials returned no access_token", 502);
  return data.access_token;
}

// Stage 2: member login with email + password -> token pair
export async function login(email: string, password: string, locale = "es"): Promise<TokenPair> {
  const tempToken = await clientCredentials();
  const data = (await request(`${BASE_URL}/api/v2.0/${locale}/exerp/newAuth`, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams({
      access_token: tempToken,
      email,
      password,
      appName: APP_NAME,
    }),
  })) as TokenResponse;
  if (!data.access_token) throw new VivaGymError("login returned no access_token", 502);
  return {
    accessToken: data.access_token,
    refreshToken: data.refresh_token ?? "",
    expiresIn: data.expires_in ?? 600,
  };
}

// Renew the access token using the refresh token
export async function refreshTokens(refreshToken: string): Promise<TokenPair> {
  const data = (await request(
    `${BASE_URL}/api/email/refresh?refresh_token=${encodeURIComponent(refreshToken)}`,
  )) as TokenResponse;
  if (!data.access_token) throw new VivaGymError("refresh returned no access_token", 502);
  return {
    accessToken: data.access_token,
    refreshToken: data.refresh_token ?? refreshToken,
    expiresIn: data.expires_in ?? 600,
  };
}

// Fetch the gym-entry QR payload for the given access token
export async function fetchQr(accessToken: string): Promise<string> {
  const text = await (async () => {
    const res = await fetch(`${BASE_URL}/api/v2.0/exerp/qr`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    const body = await res.text();
    if (!res.ok) throw new VivaGymError(`QR request failed: ${res.status} ${body.slice(0, 200)}`, res.status);
    return body;
  })();
  // The API returns a JSON-encoded string, e.g. "exerp:checkin:..."
  try {
    return JSON.parse(text) as string;
  } catch {
    return text;
  }
}
