import type {
  AuthRequest,
  OAuthHelpers,
} from "@cloudflare/workers-oauth-provider";

import {
  authCookie,
  clearAuthCookie,
  cookieValue,
  escapeHtml,
  isAllowedGitHubUser,
  pkceChallenge,
  randomToken,
  signState,
  type PendingAuthorization,
  verifySignedState,
} from "./security";

interface Env {
  OAUTH_KV: KVNamespace;
  OAUTH_PROVIDER: OAuthHelpers;
  GITHUB_CLIENT_ID: string;
  GITHUB_CLIENT_SECRET: string;
  COOKIE_ENCRYPTION_KEY: string;
  ALLOWED_GITHUB_USERS: string;
}

interface GitHubUser {
  id: number;
  login: string;
}

const STATE_PREFIX = "intervals-auth:";

export const authHandler: ExportedHandler<Env> = {
  async fetch(request, env): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/authorize" && request.method === "GET") {
      return beginAuthorization(request, env);
    }
    if (url.pathname === "/callback" && request.method === "GET") {
      return githubCallback(request, env);
    }
    if (url.pathname === "/consent" && request.method === "POST") {
      return finishConsent(request, env);
    }
    if (url.pathname === "/" && request.method === "GET") {
      return new Response(
        "Intervals.icu MCP server. Connect an MCP client to /mcp.\n",
        { headers: { "content-type": "text/plain; charset=utf-8" } },
      );
    }
    return new Response("Not found", { status: 404 });
  },
};

async function beginAuthorization(
  request: Request,
  env: Env,
): Promise<Response> {
  let oauthRequest: AuthRequest;
  try {
    oauthRequest = await env.OAUTH_PROVIDER.parseAuthRequest(request);
  } catch {
    return new Response("Invalid authorization request", { status: 400 });
  }
  const state = randomToken();
  const verifier = randomToken(48);
  const pending: PendingAuthorization = { oauthRequest, verifier };
  await env.OAUTH_KV.put(`${STATE_PREFIX}${state}`, JSON.stringify(pending), {
    expirationTtl: 600,
  });
  const signedState = await signState(state, env.COOKIE_ENCRYPTION_KEY);
  const callback = new URL("/callback", request.url);
  const github = new URL("https://github.com/login/oauth/authorize");
  github.searchParams.set("client_id", env.GITHUB_CLIENT_ID);
  github.searchParams.set("redirect_uri", callback.href);
  github.searchParams.set("scope", "read:user");
  github.searchParams.set("state", state);
  github.searchParams.set("code_challenge", await pkceChallenge(verifier));
  github.searchParams.set("code_challenge_method", "S256");
  return new Response(null, {
    status: 302,
    headers: {
      Location: github.href,
      "Set-Cookie": authCookie(signedState, callback.protocol === "https:"),
      "Cache-Control": "no-store",
    },
  });
}

async function githubCallback(request: Request, env: Env): Promise<Response> {
  const url = new URL(request.url);
  const state = url.searchParams.get("state");
  const cookieState = await verifySignedState(
    cookieValue(request, "intervals_auth"),
    env.COOKIE_ENCRYPTION_KEY,
  );
  if (!state || cookieState !== state)
    return new Response("Invalid OAuth state", { status: 400 });
  const pending = await getPending(env, state);
  const code = url.searchParams.get("code");
  if (!pending || !code)
    return new Response("Expired or invalid OAuth callback", { status: 400 });

  const tokenResponse = await fetch(
    "https://github.com/login/oauth/access_token",
    {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/x-www-form-urlencoded",
      },
      body: new URLSearchParams({
        client_id: env.GITHUB_CLIENT_ID,
        client_secret: env.GITHUB_CLIENT_SECRET,
        code,
        code_verifier: pending.verifier,
      }),
    },
  );
  const tokenBody = (await tokenResponse.json()) as { access_token?: string };
  if (!tokenResponse.ok || !tokenBody.access_token) {
    return new Response("GitHub authentication failed", { status: 502 });
  }
  const userResponse = await fetch("https://api.github.com/user", {
    headers: {
      Accept: "application/vnd.github+json",
      Authorization: `Bearer ${tokenBody.access_token}`,
      "User-Agent": "intervals-mcp-oauth-worker",
      "X-GitHub-Api-Version": "2022-11-28",
    },
  });
  const user = (await userResponse.json()) as GitHubUser;
  if (!userResponse.ok || !user.login || !Number.isSafeInteger(user.id)) {
    return new Response("GitHub identity lookup failed", { status: 502 });
  }
  if (!isAllowedGitHubUser(user.login, env.ALLOWED_GITHUB_USERS)) {
    await env.OAUTH_KV.delete(`${STATE_PREFIX}${state}`);
    return new Response(
      "This GitHub user is not allowed to access this Intervals.icu MCP server",
      { status: 403 },
    );
  }
  await env.OAUTH_KV.put(
    `${STATE_PREFIX}${state}`,
    JSON.stringify({ ...pending, githubLogin: user.login, githubId: user.id }),
    { expirationTtl: 600 },
  );
  const client = await env.OAUTH_PROVIDER.lookupClient(
    pending.oauthRequest.clientId,
  );
  if (!client) return new Response("Unknown OAuth client", { status: 400 });
  return html(
    `<!doctype html><html lang="en"><meta charset="utf-8"><title>Authorize Intervals.icu MCP</title>
    <body><main><h1>Authorize Intervals.icu MCP</h1><p>Signed in as <strong>${escapeHtml(user.login)}</strong>.</p>
    <p>Allow <strong>${escapeHtml(client.clientName ?? "this MCP client")}</strong> read-only access
    to your Intervals.icu training data?</p>
    <form method="post" action="/consent"><input type="hidden" name="state" value="${escapeHtml(state)}">
    <button name="decision" value="approve">Allow</button>
    <button name="decision" value="deny">Deny</button></form></main></body></html>`,
  );
}

async function finishConsent(request: Request, env: Env): Promise<Response> {
  const form = await request.formData();
  const state = String(form.get("state") ?? "");
  const cookieState = await verifySignedState(
    cookieValue(request, "intervals_auth"),
    env.COOKIE_ENCRYPTION_KEY,
  );
  if (!state || state !== cookieState)
    return new Response("Invalid consent state", { status: 400 });
  const pending = await getPending(env, state);
  await env.OAUTH_KV.delete(`${STATE_PREFIX}${state}`);
  if (!pending?.githubLogin || pending.githubId === undefined) {
    return new Response("Consent session expired", { status: 400 });
  }
  if (form.get("decision") !== "approve") {
    const redirect = new URL(pending.oauthRequest.redirectUri);
    redirect.searchParams.set("error", "access_denied");
    redirect.searchParams.set(
      "error_description",
      "The user denied the authorization request",
    );
    redirect.searchParams.set("state", pending.oauthRequest.state);
    return new Response(null, {
      status: 302,
      headers: {
        Location: redirect.href,
        "Set-Cookie": clearAuthCookie(
          new URL(request.url).protocol === "https:",
        ),
        "Cache-Control": "no-store",
      },
    });
  }
  const scope = pending.oauthRequest.scope.filter(
    (value) => value === "mcp:read",
  );
  const { redirectTo } = await env.OAUTH_PROVIDER.completeAuthorization({
    request: pending.oauthRequest,
    userId: String(pending.githubId),
    metadata: { githubLogin: pending.githubLogin },
    scope,
    props: { githubId: pending.githubId, githubLogin: pending.githubLogin },
  });
  return new Response(null, {
    status: 302,
    headers: {
      Location: redirectTo,
      "Set-Cookie": clearAuthCookie(new URL(request.url).protocol === "https:"),
      "Cache-Control": "no-store",
    },
  });
}

async function getPending(
  env: Env,
  state: string,
): Promise<PendingAuthorization | null> {
  return env.OAUTH_KV.get<PendingAuthorization>(
    `${STATE_PREFIX}${state}`,
    "json",
  );
}

// The CSP deliberately omits form-action: adding it breaks the deny redirect
// back to the OAuth client.
function html(body: string): Response {
  return new Response(body, {
    headers: {
      "Content-Type": "text/html; charset=utf-8",
      "Cache-Control": "no-store",
      "Content-Security-Policy":
        "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'",
      "Referrer-Policy": "no-referrer",
      "X-Content-Type-Options": "nosniff",
    },
  });
}
