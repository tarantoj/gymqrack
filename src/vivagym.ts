const BASE_URL = "https://vivagym.myvitale.com";

// Hardcoded in the app (com.vitale.coredata.BuildConfig)
const CLIENT_ID = "4_43uq8rgou3y88ckkk0sgg8c408w4gwsssg8owg0ow4wcocgw0w";
const CLIENT_SECRET = "1uiljdab2misc4owsc0kg0cw0kgw0k0gkgk0k8k488w8sskk4s";

const APP_NAME = "vivagym";

export interface VivaGymClientOptions {
  email: string;
  password: string;
  locale?: string;
}

interface TokenResponse {
  access_token?: string;
  refresh_token?: string;
  expires_in?: number;
}

export class VivaGymClient {
  private readonly email: string;
  private readonly password: string;
  private readonly locale: string;

  private accessToken: string | null = null;
  private refreshToken: string | null = null;
  private accessTokenExpiresAt = 0;

  constructor({ email, password, locale = "es" }: VivaGymClientOptions) {
    this.email = email;
    this.password = password;
    this.locale = locale;
  }

  isTokenValid(): boolean {
    return Boolean(this.accessToken) && Date.now() < this.accessTokenExpiresAt - 10_000;
  }

  // Stage 1: anonymous client_credentials token
  private async clientToken(): Promise<string> {
    const res = await fetch(`${BASE_URL}/oauth/v2/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        grant_type: "client_credentials",
        client_id: CLIENT_ID,
        client_secret: CLIENT_SECRET,
      }),
    });
    if (!res.ok) {
      throw new Error(`client_credentials failed: ${res.status} ${await res.text()}`);
    }
    const data = (await res.json()) as TokenResponse;
    if (!data.access_token) {
      throw new Error("client_credentials returned no access_token");
    }
    return data.access_token;
  }

  // Stage 2: user login with email + password -> access/refresh tokens
  async authenticate(): Promise<void> {
    const tempToken = await this.clientToken();
    const body = new URLSearchParams({
      access_token: tempToken,
      email: this.email,
      password: this.password,
      appName: APP_NAME,
    });
    const res = await fetch(`${BASE_URL}/api/v2.0/${this.locale}/exerp/newAuth`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
    });
    if (!res.ok) {
      throw new Error(`login failed: ${res.status} ${await res.text()}`);
    }
    const data = (await res.json()) as TokenResponse;
    if (!data.access_token) {
      throw new Error(`login returned no access_token: ${JSON.stringify(data)}`);
    }
    this.accessToken = data.access_token;
    this.refreshToken = data.refresh_token ?? null;
    this.accessTokenExpiresAt = Date.now() + (data.expires_in ?? 600) * 1000;
  }

  private async refresh(): Promise<boolean> {
    if (!this.refreshToken) return false;
    const res = await fetch(
      `${BASE_URL}/api/email/refresh?refresh_token=${encodeURIComponent(this.refreshToken)}`,
    );
    if (!res.ok) return false;
    const data = (await res.json()) as TokenResponse;
    if (!data.access_token) return false;
    this.accessToken = data.access_token;
    if (data.refresh_token) this.refreshToken = data.refresh_token;
    this.accessTokenExpiresAt = Date.now() + (data.expires_in ?? 600) * 1000;
    return true;
  }

  private async getAccessToken(): Promise<string> {
    if (this.isTokenValid()) return this.accessToken!;
    if (await this.refresh()) return this.accessToken!;
    await this.authenticate();
    return this.accessToken!;
  }

  async getQrPayload(): Promise<string> {
    const token = await this.getAccessToken();
    const res = await fetch(`${BASE_URL}/api/v2.0/exerp/qr`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (res.status === 401) {
      await this.authenticate();
      return this.getQrPayload();
    }
    if (!res.ok) {
      throw new Error(`QR request failed: ${res.status} ${await res.text()}`);
    }
    const text = await res.text();
    // The API returns a JSON-encoded string, e.g. "exerp:checkin:..."
    try {
      return JSON.parse(text) as string;
    } catch {
      return text;
    }
  }
}
