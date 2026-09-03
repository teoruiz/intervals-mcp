import { Container, ContainerProxy } from "@cloudflare/containers";
import {
  OAuthProvider,
  type OAuthHelpers,
} from "@cloudflare/workers-oauth-provider";
import { DurableObject, WorkerEntrypoint } from "cloudflare:workers";

import { authHandler } from "./auth";
import { buildContainerRequest } from "./security";

interface AuthProps {
  githubId: number;
  githubLogin: string;
}

interface Env {
  INTERVALS_CONTAINER: DurableObjectNamespace<IntervalsContainer>;
  OAUTH_KV: KVNamespace;
  OAUTH_PROVIDER: OAuthHelpers;
  GITHUB_CLIENT_ID: string;
  GITHUB_CLIENT_SECRET: string;
  COOKIE_ENCRYPTION_KEY: string;
  ALLOWED_GITHUB_USERS: string;
  INTERVALS_ICU_API_KEY: string;
  INTERVALS_ICU_ATHLETE_ID: string;
  MCP_RESOURCE_URL?: string;
}

export class IntervalsContainer extends Container<Env> {
  defaultPort = 8080;
  // Idle timeout, reset when the last in-flight request completes. The Go
  // server itself starts in ~13ms, so the only real cost of sleeping is
  // Cloudflare's container wake path. Handlers are stateless, so nothing is
  // lost when the instance sleeps.
  sleepAfter = "30s";
  enableInternet = false;
  interceptHttps = true;
  allowedHosts = ["intervals.icu"];

  // envVars is set here rather than as a field initializer so Worker secrets
  // reach the container. Go's crypto/x509 honors SSL_CERT_FILE on Linux, which
  // is what makes intercepted HTTPS to intervals.icu verify.
  constructor(ctx: DurableObject["ctx"], env: Env) {
    super(ctx, env);
    this.envVars = {
      MCP_ADDR: "0.0.0.0:8080",
      INTERVALS_ICU_API_KEY: env.INTERVALS_ICU_API_KEY,
      INTERVALS_ICU_ATHLETE_ID: env.INTERVALS_ICU_ATHLETE_ID,
      SSL_CERT_FILE: "/etc/cloudflare/certs/cloudflare-containers-ca.crt",
    };
  }

  // Separates the cost of waking a sleeping instance from the cost of serving
  // the request, so sleepAfter can be tuned against a real number. Purely
  // additive: startAndWaitForPorts is idempotent, and if it fails we fall
  // through to super.fetch(), which starts the container exactly as before.
  override async fetch(request: Request): Promise<Response> {
    const cold = !this.ctx.container?.running;
    let wakeMs = 0;
    if (cold) {
      const wakeStartedAt = Date.now();
      try {
        await this.startAndWaitForPorts();
      } catch {
        // Leave the error to super.fetch() so behaviour is unchanged.
      }
      wakeMs = Date.now() - wakeStartedAt;
    }

    const handleStartedAt = Date.now();
    const response = await super.fetch(request);
    console.log(
      JSON.stringify({
        msg: "container request",
        cold,
        wake_ms: wakeMs,
        handle_ms: Date.now() - handleStartedAt,
        status: response.status,
      }),
    );
    return response;
  }
}

export { ContainerProxy };

class McpProxyHandler extends WorkerEntrypoint<Env, AuthProps> {
  async fetch(request: Request): Promise<Response> {
    const incoming = new URL(request.url);
    if (incoming.pathname !== "/mcp")
      return new Response("Not found", { status: 404 });
    const proxied = buildContainerRequest(request, this.ctx.props.githubLogin);
    // Timing makes cold starts visible in `wrangler tail`: a request that had
    // to wake the container costs far more than one reaching a running
    // instance. Use it to re-tune sleepAfter.
    const startedAt = Date.now();
    const response =
      await this.env.INTERVALS_CONTAINER.getByName("singleton").fetch(proxied);
    console.log(
      JSON.stringify({
        msg: "mcp proxy",
        status: response.status,
        duration_ms: Date.now() - startedAt,
      }),
    );
    return response;
  }
}

function resourceUrl(request: Request, env: Env): string {
  if (env.MCP_RESOURCE_URL) {
    const configured = new URL(env.MCP_RESOURCE_URL);
    if (configured.protocol !== "https:" || configured.pathname !== "/mcp") {
      throw new Error("MCP_RESOURCE_URL must be an HTTPS URL ending in /mcp");
    }
    return configured.href;
  }
  return new URL("/mcp", request.url).href;
}

function provider(request: Request, env: Env): OAuthProvider<Env> {
  const resource = resourceUrl(request, env);
  const origin = new URL(resource).origin;
  // The OAuth provider only accepts HTTPS issuers in authorization_servers.
  // Deployments are always HTTPS; local `wrangler dev` serves http://localhost,
  // where the field is omitted so discovery still resolves.
  const issuers = origin.startsWith("https:")
    ? { authorization_servers: [origin] }
    : {};
  return new OAuthProvider<Env>({
    apiRoute: "/mcp",
    apiHandler: McpProxyHandler,
    defaultHandler: authHandler,
    authorizeEndpoint: "/authorize",
    tokenEndpoint: "/token",
    clientRegistrationEndpoint: "/register",
    clientIdMetadataDocumentEnabled: true,
    allowPlainPKCE: false,
    scopesSupported: ["mcp:read"],
    resourceMetadata: {
      resource,
      ...issuers,
      scopes_supported: ["mcp:read"],
      resource_name: "Intervals.icu MCP",
    },
  });
}

export default {
  async fetch(
    request: Request,
    env: Env,
    ctx: ExecutionContext,
  ): Promise<Response> {
    return provider(request, env).fetch(request, env, ctx);
  },
} satisfies ExportedHandler<Env>;
