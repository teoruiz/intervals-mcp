import type { AuthRequest } from "@cloudflare/workers-oauth-provider";

export interface PendingAuthorization {
  oauthRequest: AuthRequest;
  verifier: string;
  githubLogin?: string;
  githubId?: number;
}

const encoder = new TextEncoder();

export function allowedGitHubUsers(raw: string): Set<string> {
  return new Set(
    raw
      .split(",")
      .map((value) => value.trim().toLowerCase())
      .filter(Boolean),
  );
}

export function isAllowedGitHubUser(
  login: string,
  rawAllowlist: string,
): boolean {
  const allowlist = allowedGitHubUsers(rawAllowlist);
  return allowlist.size > 0 && allowlist.has(login.toLowerCase());
}

export function stripProxyHeaders(input: Headers): Headers {
  const output = new Headers(input);
  for (const name of [
    "authorization",
    "proxy-authorization",
    "cookie",
    "cf-access-jwt-assertion",
    "cf-connecting-ip",
    "true-client-ip",
    "x-forwarded-for",
    "x-forwarded-host",
    "x-forwarded-proto",
    "content-length",
  ]) {
    output.delete(name);
  }
  return output;
}

export function buildContainerRequest(
  request: Request,
  authenticatedUser: string,
): Request {
  const headers = stripProxyHeaders(request.headers);
  headers.set("x-intervals-authenticated-user", authenticatedUser);
  return new Request("http://localhost/mcp", {
    method: request.method,
    headers,
    body: request.body,
    redirect: "manual",
  });
}

export function randomToken(bytes = 32): string {
  const raw = new Uint8Array(bytes);
  crypto.getRandomValues(raw);
  return base64Url(raw);
}

export async function pkceChallenge(verifier: string): Promise<string> {
  return base64Url(
    new Uint8Array(
      await crypto.subtle.digest("SHA-256", encoder.encode(verifier)),
    ),
  );
}

export async function signState(
  state: string,
  secret: string,
): Promise<string> {
  const key = await crypto.subtle.importKey(
    "raw",
    encoder.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["sign"],
  );
  const signature = await crypto.subtle.sign(
    "HMAC",
    key,
    encoder.encode(state),
  );
  return `${state}.${base64Url(new Uint8Array(signature))}`;
}

export async function verifySignedState(
  value: string | null,
  secret: string,
): Promise<string | null> {
  if (!value) return null;
  const separator = value.lastIndexOf(".");
  if (separator <= 0) return null;
  const state = value.slice(0, separator);
  const signature = value.slice(separator + 1);
  let signatureBytes: Uint8Array;
  try {
    signatureBytes = fromBase64Url(signature);
  } catch {
    return null;
  }
  const key = await crypto.subtle.importKey(
    "raw",
    encoder.encode(secret),
    { name: "HMAC", hash: "SHA-256" },
    false,
    ["verify"],
  );
  const valid = await crypto.subtle.verify(
    "HMAC",
    key,
    signatureBytes as BufferSource,
    encoder.encode(state),
  );
  return valid ? state : null;
}

export function cookieValue(request: Request, name: string): string | null {
  for (const part of (request.headers.get("Cookie") ?? "").split(";")) {
    const [key, ...rest] = part.trim().split("=");
    if (key === name) return rest.join("=");
  }
  return null;
}

export function authCookie(value: string, secure = true): string {
  return `intervals_auth=${value}; Path=/; HttpOnly; SameSite=Lax; Max-Age=600${secure ? "; Secure" : ""}`;
}

export function clearAuthCookie(secure = true): string {
  return `intervals_auth=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0${secure ? "; Secure" : ""}`;
}

export function escapeHtml(value: string): string {
  return value.replace(
    /[&<>"']/g,
    (character) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        character
      ]!,
  );
}

function base64Url(value: Uint8Array): string {
  let binary = "";
  for (const byte of value) binary += String.fromCharCode(byte);
  return btoa(binary)
    .replaceAll("+", "-")
    .replaceAll("/", "_")
    .replace(/=+$/, "");
}

function fromBase64Url(value: string): Uint8Array {
  const normalized = value.replaceAll("-", "+").replaceAll("_", "/");
  const binary = atob(
    normalized + "=".repeat((4 - (normalized.length % 4)) % 4),
  );
  return Uint8Array.from(binary, (character) => character.charCodeAt(0));
}
