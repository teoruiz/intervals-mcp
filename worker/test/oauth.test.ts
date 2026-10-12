import {
  OAuthProvider,
  type AuthRequest,
  type OAuthHelpers,
} from "@cloudflare/workers-oauth-provider";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { authHandler } from "../src/auth";
import { pkceChallenge, signState } from "../src/security";

// Only the runtime base class is stubbed; grants, PKCE, encryption, revocation
// and token rotation all run through the installed OAuth provider.
vi.mock("cloudflare:workers", () => ({ WorkerEntrypoint: class {} }));

class MemoryKv {
  private values = new Map<string, { value: string; expiresAt?: number }>();

  async put(
    key: string,
    value: string,
    options?: { expirationTtl?: number },
  ): Promise<void> {
    this.values.set(key, {
      value,
      expiresAt: options?.expirationTtl
        ? Date.now() + options.expirationTtl * 1000
        : undefined,
    });
  }

  async get<T>(
    key: string,
    type: "json" | { type: "json" },
  ): Promise<T | null> {
    const stored = this.values.get(key);
    if (!stored) return null;
    if (stored.expiresAt !== undefined && stored.expiresAt <= Date.now()) {
      this.values.delete(key);
      return null;
    }
    return JSON.parse(stored.value) as T;
  }

  async delete(key: string): Promise<void> {
    this.values.delete(key);
  }

  async list({ prefix }: { prefix: string }) {
    return {
      keys: [...this.values.keys()]
        .filter((key) => key.startsWith(prefix))
        .map((name) => ({ name })),
      list_complete: true,
    };
  }
}

const clientId = "https://chatgpt.com/oauth/client.json";
const redirectUri = "https://chatgpt.com/connector_platform_oauth_redirect";
const verifier = "test-pkce-verifier-with-at-least-43-characters-long";
const context = { waitUntil: vi.fn() } as unknown as ExecutionContext;

function service(origin: string, kv: MemoryKv) {
  const resource = `${origin}/mcp`;
  const env = {
    OAUTH_KV: kv as unknown as KVNamespace,
    OAUTH_PROVIDER: undefined as unknown as OAuthHelpers,
    GITHUB_CLIENT_ID: "test-client",
    GITHUB_CLIENT_SECRET: "test-secret",
    COOKIE_ENCRYPTION_KEY: "test-cookie-signing-key",
    ALLOWED_GITHUB_USERS: "alice",
  };
  const provider = new OAuthProvider({
    apiRoute: "/mcp",
    apiHandler: { fetch: async () => Response.json({ authenticated: true }) },
    defaultHandler: authHandler,
    authorizeEndpoint: "/authorize",
    tokenEndpoint: "/token",
    clientIdMetadataDocumentEnabled: true,
    allowPlainPKCE: false,
    resourceMetadata: { resource, authorization_servers: [origin] },
    onError: () => {},
  });
  return {
    env,
    resource,
    fetch: (path: string, init?: RequestInit) =>
      provider.fetch(new Request(`${origin}${path}`, init), env, context),
    async authRequest(): Promise<AuthRequest> {
      // An ordinary request installs the real OAuth helpers in this env.
      await this.fetch("/");
      const params = new URLSearchParams({
        client_id: clientId,
        redirect_uri: redirectUri,
        response_type: "code",
        scope: "mcp:read",
        state: "client-state",
        code_challenge: await pkceChallenge(verifier),
        code_challenge_method: "S256",
        resource,
      });
      return env.OAUTH_PROVIDER!.parseAuthRequest(
        new Request(`${origin}/authorize?${params}`),
      );
    },
    token(params: Record<string, string>) {
      return this.fetch("/token", {
        method: "POST",
        body: new URLSearchParams({ client_id: clientId, resource, ...params }),
      });
    },
    async connect(beforeExchange?: () => Promise<void>) {
      const oauthRequest = await this.authRequest();
      const state = "browser-state";
      await kv.put(
        `intervals-auth:${state}`,
        JSON.stringify({
          oauthRequest,
          verifier: "upstream-verifier",
          githubLogin: "alice",
          githubId: 42,
        }),
      );
      const cookie = await signState(state, env.COOKIE_ENCRYPTION_KEY);
      const consent = await this.fetch("/consent", {
        method: "POST",
        headers: { cookie: `intervals_auth=${cookie}` },
        body: new URLSearchParams({ state, decision: "approve" }),
      });
      expect(consent.status).toBe(302);
      const redirect = new URL(consent.headers.get("location")!);
      expect(redirect.searchParams.get("iss")).toBe(origin);
      expect(redirect.searchParams.get("state")).toBe("client-state");
      await beforeExchange?.();
      const response = await this.token({
        grant_type: "authorization_code",
        code: redirect.searchParams.get("code")!,
        redirect_uri: redirectUri,
        code_verifier: verifier,
      });
      expect(response.status).toBe(200);
      return (await response.json()) as {
        access_token: string;
        refresh_token: string;
      };
    },
  };
}

describe("OAuth grant isolation and refresh", () => {
  beforeEach(() => {
    vi.stubGlobal("Cloudflare", {
      compatibilityFlags: { global_fetch_strictly_public: true },
    });
    vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
      Response.json({
        client_id: clientId,
        client_name: "ChatGPT",
        redirect_uris: [redirectUri],
        token_endpoint_auth_method: "none",
        grant_types: ["authorization_code", "refresh_token"],
        response_types: ["code"],
      }),
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("keeps a grant refreshable when another service authorizes the same GitHub user and CIMD client", async () => {
    const kv = new MemoryKv();
    const intervals = service("https://intervals.example", kv);
    const other = service("https://other.example", kv);
    const tokens = await intervals.connect();

    // The other Worker still uses the original bare GitHub ID and shares KV,
    // client metadata and the redirect URI with Intervals.
    const otherRequest = await other.authRequest();
    await other.env.OAUTH_PROVIDER!.completeAuthorization({
      request: otherRequest,
      userId: "42",
      metadata: {},
      scope: ["mcp:read"],
      props: { githubId: 42, githubLogin: "alice" },
    });

    const refreshed = await intervals.token({
      grant_type: "refresh_token",
      refresh_token: tokens.refresh_token,
    });
    expect(refreshed.status).toBe(200);
    const rotated = (await refreshed.json()) as { access_token: string };
    const authenticated = await intervals.fetch("/mcp", {
      headers: { authorization: `Bearer ${rotated.access_token}` },
    });
    expect(authenticated.status).toBe(200);
    const crossService = await other.fetch("/mcp", {
      headers: { authorization: `Bearer ${rotated.access_token}` },
    });
    expect(crossService.status).toBe(401);
  });

  it("refreshes after two days and tolerates retrying the previous refresh token", async () => {
    const intervals = service("https://intervals.example", new MemoryKv());
    const tokens = await intervals.connect();
    const later = Date.now() + 2 * 24 * 60 * 60 * 1000;
    vi.spyOn(Date, "now").mockReturnValue(later);
    const expired = await intervals.fetch("/mcp", {
      headers: { authorization: `Bearer ${tokens.access_token}` },
    });
    expect(expired.status).toBe(401);

    let rotated!: { access_token: string; refresh_token: string };
    for (let attempt = 0; attempt < 2; attempt++) {
      const refreshed = await intervals.token({
        grant_type: "refresh_token",
        refresh_token: tokens.refresh_token,
      });
      expect(refreshed.status).toBe(200);
      rotated = (await refreshed.json()) as {
        access_token: string;
        refresh_token: string;
      };
      expect(rotated.refresh_token).not.toBe(tokens.refresh_token);
      expect(
        (
          await intervals.fetch("/mcp", {
            headers: { authorization: `Bearer ${rotated.access_token}` },
          })
        ).status,
      ).toBe(200);
    }
    const next = await intervals.token({
      grant_type: "refresh_token",
      refresh_token: rotated.refresh_token,
    });
    expect(next.status).toBe(200);
  });

  it("keeps the authorization code valid if another service authorizes before ChatGPT exchanges it", async () => {
    const kv = new MemoryKv();
    const intervals = service("https://intervals.example", kv);
    const other = service("https://other.example", kv);
    const tokens = await intervals.connect(async () => {
      const request = await other.authRequest();
      await other.env.OAUTH_PROVIDER!.completeAuthorization({
        request,
        userId: "42",
        metadata: {},
        scope: ["mcp:read"],
        props: { githubId: 42, githubLogin: "alice" },
      });
    });
    expect(
      (
        await intervals.fetch("/mcp", {
          headers: { authorization: `Bearer ${tokens.access_token}` },
        })
      ).status,
    ).toBe(200);
  });

  it("keeps both ChatGPT connections usable when they share a client ID and callback", async () => {
    const intervals = service("https://intervals.example", new MemoryKv());
    const desktop = await intervals.connect();
    const mobile = await intervals.connect();
    expect(mobile.refresh_token).not.toBe(desktop.refresh_token);

    // Connecting on mobile must preserve the existing desktop access token.
    expect(
      (
        await intervals.fetch("/mcp", {
          headers: { authorization: `Bearer ${desktop.access_token}` },
        })
      ).status,
    ).toBe(200);

    const later = Date.now() + 2 * 24 * 60 * 60 * 1000;
    vi.spyOn(Date, "now").mockReturnValue(later);
    const refresh = async (refreshToken: string) => {
      const response = await intervals.token({
        grant_type: "refresh_token",
        refresh_token: refreshToken,
      });
      expect(response.status).toBe(200);
      const tokens = (await response.json()) as {
        access_token: string;
        refresh_token: string;
      };
      expect(
        (
          await intervals.fetch("/mcp", {
            headers: { authorization: `Bearer ${tokens.access_token}` },
          })
        ).status,
      ).toBe(200);
      return tokens;
    };

    // Multiple rotations on one connection must not invalidate the other's
    // untouched refresh token, or its next generation after it does refresh.
    const desktopFirst = await refresh(desktop.refresh_token);
    await refresh(desktopFirst.refresh_token);
    const mobileFirst = await refresh(mobile.refresh_token);
    const mobileSecond = await refresh(mobileFirst.refresh_token);
    await refresh(mobileSecond.refresh_token);
  });

  it("keeps an in-flight authorization code valid when another connection consents", async () => {
    const intervals = service("https://intervals.example", new MemoryKv());
    let second!: { access_token: string; refresh_token: string };
    const first = await intervals.connect(async () => {
      second = await intervals.connect();
    });
    for (const tokens of [first, second]) {
      expect(
        (
          await intervals.fetch("/mcp", {
            headers: { authorization: `Bearer ${tokens.access_token}` },
          })
        ).status,
      ).toBe(200);
    }
  });

  it("still permits explicitly revoking one connection without revoking another", async () => {
    const intervals = service("https://intervals.example", new MemoryKv());
    const first = await intervals.connect();
    const second = await intervals.connect();
    const beforeRevocation = await intervals.token({
      grant_type: "refresh_token",
      refresh_token: first.refresh_token,
    });
    expect(beforeRevocation.status).toBe(200);
    const revoked = await intervals.token({
      token: first.refresh_token,
      token_type_hint: "refresh_token",
    });
    expect(revoked.status).toBe(200);
    const old = await intervals.token({
      grant_type: "refresh_token",
      refresh_token: first.refresh_token,
    });
    expect(old.status).toBe(400);
    const active = await intervals.token({
      grant_type: "refresh_token",
      refresh_token: second.refresh_token,
    });
    expect(active.status).toBe(200);
  });
});
