package runtime

import (
	"encoding/json"
	"net/http"
	"strings"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/teoruiz/intervals-mcp/internal/adapters/httpauth"
	"github.com/teoruiz/intervals-mcp/internal/adapters/oauthui"
	"github.com/teoruiz/intervals-mcp/internal/platform/config"
)

func NewAuthenticatedHTTPHandler(cfg config.Config, httpClient *http.Client) (http.Handler, error) {
	mcpHandler, err := NewMCPHTTPHandler(cfg, httpClient)
	if err != nil {
		return nil, err
	}

	verifier, err := httpauth.NewJWTVerifier(httpauth.VerifierConfig{
		IssuerURL:      cfg.OIDCIssuerURL,
		Audience:       cfg.OIDCAudience,
		AllowedEmail:   cfg.OIDCAllowedEmail,
		AllowedSubject: cfg.OIDCAllowedSub,
		JWKSURL:        cfg.OIDCJWKSURL,
		HTTPClient:     httpClient,
	})
	if err != nil {
		return nil, err
	}

	oauthHandler, err := oauthui.New(oauthui.Config{
		SupabaseURL:     cfg.SupabaseURL,
		SupabaseAnonKey: cfg.SupabaseAnonKey,
		Providers:       cfg.SupabaseOAuthProviders,
	})
	if err != nil {
		return nil, err
	}

	protectedMCP := mcpauth.RequireBearerToken(verifier.Verify, &mcpauth.RequireBearerTokenOptions{
		ResourceMetadataURL: cfg.ResourceMetadataURL(),
		Scopes:              cfg.RequiredScopes,
	})(mcpHandler)

	mux := http.NewServeMux()
	mux.Handle("POST /mcp", protectedMCP)
	mux.Handle("GET /mcp", protectedMCP)
	mux.Handle("DELETE /mcp", protectedMCP)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", protectedResourceHandler(cfg))
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp", protectedResourceHandler(cfg))
	mux.Handle("GET /oauth/consent", oauthHandler)
	mux.Handle("GET /oauth/callback", oauthHandler)
	registerHealthHandlers(mux)
	return mux, nil
}

func NewLocalHTTPHandler(cfg config.Config, httpClient *http.Client) (http.Handler, error) {
	mcpHandler, err := NewMCPHTTPHandler(cfg, httpClient)
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("POST /mcp", mcpHandler)
	mux.Handle("GET /mcp", mcpHandler)
	mux.Handle("DELETE /mcp", mcpHandler)
	registerHealthHandlers(mux)
	return mux, nil
}

func registerHealthHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /readyz", readyHandler)
}

func protectedResourceHandler(cfg config.Config) http.HandlerFunc {
	metadata := oauthex.ProtectedResourceMetadata{
		Resource:               cfg.MCPResource(),
		AuthorizationServers:   []string{cfg.OIDCIssuerURL},
		ScopesSupported:        cfg.RequiredScopes,
		BearerMethodsSupported: []string{"header"},
		ResourceName:           "Intervals.icu MCP",
		ResourceDocumentation:  strings.TrimRight(cfg.MCPPublicURL, "/"),
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(metadata); err != nil {
			http.Error(w, "encode metadata", http.StatusInternalServerError)
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}
