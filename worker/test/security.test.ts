import type { AuthRequest } from "@cloudflare/workers-oauth-provider";
import { afterEach, describe, expect, it, vi } from "vitest";

import { authHandler } from "../src/auth";
import {
  allowedGitHubUsers,
  buildContainerRequest,
  isAllowedGitHubUser,
  signState,
  stripProxyHeaders,
  verifySignedState,
} from "../src/security";

class MemoryKv {
  values = new Map<string, string>();

  async put(key: string, value: string): Promise<void> {
    this.values.set(key, value);
  }

  async get<T>(key: string, type: "json"): Promise<T | null> {
    const value = this.values.get(key);
    return value === undefined ? null : (JSON.parse(value) as T);
  }

  async delete(key: string): Promise<void> {
    this.values.delete(key);
  }
}

function authRequest(): AuthRequest {
  return {
    clientId: "https://client.example/metadata.json",
    redirectUri: "https://client.example/callback",
    responseType: "code",
    state: "client-state",
    scope: ["mcp:read"],
    codeChallenge: "client-challenge",
    codeChallengeMethod: "S256",
  };
}

function environment(kv: MemoryKv, request = authRequest()) {
  return {
    OAUTH_KV: kv as unknown as KVNamespace,
    OAUTH_PROVIDER: {
      parseAuthRequest: async () => request,
    },
    GITHUB_CLIENT_ID: "github-client",
    GITHUB_CLIENT_SECRET: "github-secret",
    COOKIE_ENCRYPTION_KEY: "a-long-cookie-signing-secret",
    ALLOWED_GITHUB_USERS: "alice",
  };
}

describe("worker security helpers", () => {
  it("normalizes and fails closed on the GitHub allowlist", () => {
    expect([...allowedGitHubUsers(" TeoRuiz,alice ")]).toEqual([
      "teoruiz",
      "alice",
    ]);
    expect(isAllowedGitHubUser("TEORUIZ", "teoruiz")).toBe(true);
    expect(isAllowedGitHubUser("teoruiz", "")).toBe(false);
  });

  it("removes credentials and forwarding identity before proxying", () => {
    const headers = stripProxyHeaders(
      new Headers({
        authorization: "Bearer secret",
        cookie: "session=secret",
        "cf-connecting-ip": "192.0.2.1",
        "mcp-protocol-version": "2026-07-28",
        "content-type": "application/json",
      }),
    );
    expect(headers.has("authorization")).toBe(false);
    expect(headers.has("cookie")).toBe(false);
    expect(headers.has("cf-connecting-ip")).toBe(false);
    expect(headers.get("mcp-protocol-version")).toBe("2026-07-28");
  });

  it("builds an exact container request and overwrites caller identity", () => {
    const request = buildContainerRequest(
      new Request("https://intervals.example/mcp?ignored=true", {
        headers: {
          authorization: "Bearer secret",
          "x-intervals-authenticated-user": "mallory",
        },
      }),
      "alice",
    );
    expect(request.url).toBe("http://localhost/mcp");
    expect(request.headers.has("authorization")).toBe(false);
    expect(request.headers.get("x-intervals-authenticated-user")).toBe("alice");
    expect(request.method).toBe("GET");
  });

  it("signs browser state and rejects tampering", async () => {
    const value = await signState("state", "a sufficiently long test secret");
    expect(
      await verifySignedState(value, "a sufficiently long test secret"),
    ).toBe("state");
    expect(
      await verifySignedState(`${value}x`, "a sufficiently long test secret"),
    ).toBeNull();
  });
});

describe("GitHub authorization handler", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("stores pending authorization and redirects with upstream state", async () => {
    const kv = new MemoryKv();
    const response = await authHandler.fetch!(
      new Request("https://intervals.example/authorize"),
      environment(kv) as never,
      {} as ExecutionContext,
    );

    expect(response.status).toBe(302);
    const location = new URL(response.headers.get("location")!);
    expect(location.origin).toBe("https://github.com");
    expect(location.searchParams.get("client_id")).toBe("github-client");
    expect(location.searchParams.get("state")).toBeTruthy();
    expect(location.searchParams.get("code_challenge_method")).toBe("S256");
    expect(response.headers.get("set-cookie")).toContain("HttpOnly");
    expect(response.headers.get("set-cookie")).toContain("SameSite=Lax");
    expect(kv.values.size).toBe(1);
  });

  it("returns a standards-shaped denial to the validated client redirect", async () => {
    const kv = new MemoryKv();
    const state = "browser-state";
    const secret = "a-long-cookie-signing-secret";
    kv.values.set(
      `intervals-auth:${state}`,
      JSON.stringify({
        oauthRequest: authRequest(),
        verifier: "verifier",
        githubLogin: "alice",
        githubId: 42,
      }),
    );
    const cookie = await signState(state, secret);
    const response = await authHandler.fetch!(
      new Request("https://intervals.example/consent", {
        method: "POST",
        headers: {
          "content-type": "application/x-www-form-urlencoded",
          cookie: `intervals_auth=${cookie}`,
        },
        body: new URLSearchParams({ state, decision: "deny" }),
      }),
      environment(kv) as never,
      {} as ExecutionContext,
    );

    expect(response.status).toBe(302);
    const location = new URL(response.headers.get("location")!);
    expect(location.href).toContain("https://client.example/callback");
    expect(location.searchParams.get("error")).toBe("access_denied");
    expect(location.searchParams.get("state")).toBe("client-state");
    expect(kv.values.size).toBe(0);
  });

  it("rejects a GitHub login that is not on the allowlist", async () => {
    const kv = new MemoryKv();
    const state = "browser-state";
    const secret = "a-long-cookie-signing-secret";
    kv.values.set(
      `intervals-auth:${state}`,
      JSON.stringify({ oauthRequest: authRequest(), verifier: "verifier" }),
    );
    const cookie = await signState(state, secret);
    const fetchMock = vi.spyOn(globalThis, "fetch");
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url === "https://github.com/login/oauth/access_token") {
        return Response.json({ access_token: "github-token" });
      }
      if (url === "https://api.github.com/user") {
        return Response.json({ id: 7, login: "mallory" });
      }
      throw new Error(`Unexpected fetch: ${url}`);
    });

    const response = await authHandler.fetch!(
      new Request(
        `https://intervals.example/callback?state=${state}&code=github-code`,
        { headers: { cookie: `intervals_auth=${cookie}` } },
      ),
      environment(kv) as never,
      {} as ExecutionContext,
    );

    expect(response.status).toBe(403);
    expect(kv.values.size).toBe(0);
  });

  it("allows the consent POST to redirect to the validated OAuth client", async () => {
    const kv = new MemoryKv();
    const state = "browser-state";
    const secret = "a-long-cookie-signing-secret";
    kv.values.set(
      `intervals-auth:${state}`,
      JSON.stringify({
        oauthRequest: authRequest(),
        verifier: "verifier",
      }),
    );
    const cookie = await signState(state, secret);
    const fetchMock = vi.spyOn(globalThis, "fetch");
    fetchMock.mockImplementation(async (input) => {
      const url = String(input);
      if (url === "https://github.com/login/oauth/access_token") {
        return Response.json({ access_token: "github-token" });
      }
      if (url === "https://api.github.com/user") {
        return Response.json({ id: 42, login: "alice" });
      }
      throw new Error(`Unexpected fetch: ${url}`);
    });
    const env = environment(kv) as ReturnType<typeof environment> & {
      OAUTH_PROVIDER: {
        parseAuthRequest: () => Promise<AuthRequest>;
        lookupClient: () => Promise<{ clientName: string }>;
      };
    };
    env.OAUTH_PROVIDER.lookupClient = async () => ({ clientName: "ChatGPT" });

    const response = await authHandler.fetch!(
      new Request(
        `https://intervals.example/callback?state=${state}&code=github-code`,
        { headers: { cookie: `intervals_auth=${cookie}` } },
      ),
      env as never,
      {} as ExecutionContext,
    );

    expect(response.status).toBe(200);
    expect(response.headers.get("content-security-policy")).not.toContain(
      "form-action",
    );
    expect(await response.text()).toContain('action="/consent"');
  });
});
