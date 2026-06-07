package oauthui

import (
	"encoding/json"
	"html/template"
	"net/http"
)

type Config struct {
	SupabaseURL     string
	SupabaseAnonKey string
	Providers       []string
}

type Handler struct {
	tmpl *template.Template
	cfg  Config
}

type viewData struct {
	SupabaseURLJSON     template.JS
	SupabaseAnonKeyJSON template.JS
	Providers           []string
}

func New(cfg Config) (*Handler, error) {
	tmpl, err := template.New("consent").Parse(consentHTML)
	if err != nil {
		return nil, err
	}
	return &Handler{tmpl: tmpl, cfg: cfg}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	data := viewData{
		SupabaseURLJSON:     mustJSON(h.cfg.SupabaseURL),
		SupabaseAnonKeyJSON: mustJSON(h.cfg.SupabaseAnonKey),
		Providers:           h.cfg.Providers,
	}
	if err := h.tmpl.Execute(w, data); err != nil {
		http.Error(w, "render consent page", http.StatusInternalServerError)
		return
	}
}

func mustJSON(value string) template.JS {
	data, err := json.Marshal(value)
	if err != nil {
		return template.JS(`""`)
	}
	return template.JS(data)
}

const consentHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Authorize Intervals MCP</title>
  <style>
    :root {
      color-scheme: light dark;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: #f5f7f8;
      color: #172026;
    }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 24px;
    }
    main {
      width: min(100%, 520px);
      background: canvas;
      border: 1px solid color-mix(in srgb, CanvasText 14%, transparent);
      border-radius: 8px;
      padding: 28px;
      box-shadow: 0 18px 50px color-mix(in srgb, CanvasText 10%, transparent);
    }
    h1 {
      margin: 0 0 10px;
      font-size: 1.35rem;
      line-height: 1.25;
      letter-spacing: 0;
    }
    p {
      margin: 0 0 18px;
      line-height: 1.5;
      color: color-mix(in srgb, CanvasText 72%, transparent);
    }
    .actions {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      margin-top: 20px;
    }
    button {
      min-height: 40px;
      border: 1px solid color-mix(in srgb, CanvasText 18%, transparent);
      background: Canvas;
      color: CanvasText;
      border-radius: 6px;
      padding: 0 14px;
      font: inherit;
      cursor: pointer;
    }
    button.primary {
      background: #176b52;
      border-color: #176b52;
      color: white;
    }
    button.danger {
      color: #b42318;
    }
    code {
      word-break: break-word;
      font-size: 0.9em;
    }
    .hidden {
      display: none;
    }
    .status {
      margin-top: 18px;
      font-size: 0.95rem;
      color: color-mix(in srgb, CanvasText 70%, transparent);
    }
    form {
      display: grid;
      gap: 12px;
      margin-top: 18px;
    }
    label {
      display: grid;
      gap: 6px;
      font-size: 0.92rem;
      color: color-mix(in srgb, CanvasText 78%, transparent);
    }
    input {
      min-height: 40px;
      border: 1px solid color-mix(in srgb, CanvasText 18%, transparent);
      border-radius: 6px;
      background: Canvas;
      color: CanvasText;
      padding: 0 10px;
      font: inherit;
    }
  </style>
</head>
<body>
  <main>
    <h1>Authorize Intervals MCP</h1>
    <p id="summary">Checking authorization request...</p>
    <div id="details" class="hidden">
      <p><strong>Client:</strong> <span id="client-name"></span></p>
      <p><strong>Scopes:</strong> <code id="scopes"></code></p>
    </div>
    <div id="login" class="hidden">
      <form id="email-login">
        <label>Email
          <input id="email" name="email" type="email" autocomplete="email" required>
        </label>
        <label>Password
          <input id="password" name="password" type="password" autocomplete="current-password" required>
        </label>
        <div class="actions">
          <button class="primary" type="submit">Sign in</button>
        </div>
      </form>
      {{if .Providers}}
      <div class="actions" id="social-login">
        {{range .Providers}}<button type="button" data-provider="{{.}}">Continue with {{.}}</button>{{end}}
      </div>
      {{end}}
    </div>
    <div id="consent" class="actions hidden">
      <button class="primary" id="approve">Allow</button>
      <button class="danger" id="deny">Deny</button>
    </div>
    <div id="status" class="status"></div>
  </main>

  <noscript>
    <style>#summary::after { content: " JavaScript is required for Supabase OAuth consent."; }</style>
  </noscript>
  <script>
    if (!new URLSearchParams(window.location.search).get("authorization_id") && !sessionStorage.getItem("authorization_id")) {
      document.querySelector("#summary").textContent = "Missing authorization_id in the request URL.";
    }
  </script>
  <script type="module">
    import { createClient } from "https://cdn.jsdelivr.net/npm/@supabase/supabase-js@2/+esm";

    const supabase = createClient({{.SupabaseURLJSON}}, {{.SupabaseAnonKeyJSON}});
    const params = new URLSearchParams(window.location.search);
    const authorizationId = params.get("authorization_id") || sessionStorage.getItem("authorization_id");
    const summary = document.querySelector("#summary");
    const status = document.querySelector("#status");
    const details = document.querySelector("#details");
    const login = document.querySelector("#login");
    const emailLogin = document.querySelector("#email-login");
    const consent = document.querySelector("#consent");

    if (authorizationId) {
      sessionStorage.setItem("authorization_id", authorizationId);
    }

    function setStatus(message) {
      status.textContent = message || "";
    }

    function redirectFrom(data) {
      return data?.redirect_to || data?.redirect_url || data?.url || "";
    }

    function finishAuthorization(data) {
      const redirect = redirectFrom(data);
      if (!redirect) {
        setStatus("Authorization succeeded but Supabase did not return a redirect URL: " + JSON.stringify(data));
        return;
      }
      window.location.href = redirect;
    }

    async function load() {
      if (!authorizationId) {
        summary.textContent = "Missing authorization_id in the request URL.";
        return;
      }

      const { data: sessionData } = await supabase.auth.getSession();
      if (!sessionData.session) {
        summary.textContent = "Sign in to authorize this MCP connection.";
        login.classList.remove("hidden");
        return;
      }

      if (!supabase.auth.oauth) {
        summary.textContent = "This Supabase project does not expose OAuth server methods to the browser client.";
        setStatus("Check that the Supabase OAuth server is enabled for this project.");
        return;
      }

      const { data, error } = await supabase.auth.oauth.getAuthorizationDetails(authorizationId);
      if (error) {
        summary.textContent = "Could not load authorization details.";
        setStatus(error.message);
        return;
      }
      const alreadyApprovedRedirect = redirectFrom(data);
      if (alreadyApprovedRedirect && !("authorization_id" in data)) {
        window.location.href = alreadyApprovedRedirect;
        return;
      }

      summary.textContent = "Allow this client to access your read-only Intervals MCP server?";
      document.querySelector("#client-name").textContent = data?.client?.client_name || data?.client_id || "MCP client";
      const requestedScopes = data?.scopes || data?.scope?.split?.(" ") || [];
      document.querySelector("#scopes").textContent = requestedScopes.filter(Boolean).join(" ") || "none";
      details.classList.remove("hidden");
      consent.classList.remove("hidden");
    }

    emailLogin.addEventListener("submit", async (event) => {
      event.preventDefault();
      setStatus("Signing in...");
      const form = new FormData(emailLogin);
      const email = String(form.get("email") || "");
      const password = String(form.get("password") || "");
      const { error } = await supabase.auth.signInWithPassword({ email, password });
      if (error) {
        setStatus(error.message);
        return;
      }
      login.classList.add("hidden");
      setStatus("");
      await load();
    });

    login.addEventListener("click", async (event) => {
      const provider = event.target?.dataset?.provider;
      if (!provider) return;
      setStatus("Redirecting...");
      const redirectTo = window.location.origin + "/oauth/consent?authorization_id=" + encodeURIComponent(authorizationId);
      const { error } = await supabase.auth.signInWithOAuth({ provider, options: { redirectTo } });
      if (error) setStatus(error.message);
    });

    document.querySelector("#approve").addEventListener("click", async () => {
      setStatus("Approving...");
      const { data, error } = await supabase.auth.oauth.approveAuthorization(authorizationId);
      if (error) {
        setStatus(error.message);
        return;
      }
      finishAuthorization(data);
    });

    document.querySelector("#deny").addEventListener("click", async () => {
      setStatus("Denying...");
      const { data, error } = await supabase.auth.oauth.denyAuthorization(authorizationId);
      if (error) {
        setStatus(error.message);
        return;
      }
      finishAuthorization(data);
    });

    load().catch((error) => {
      summary.textContent = "Authorization failed.";
      setStatus(error.message);
    });
  </script>
</body>
</html>
`
