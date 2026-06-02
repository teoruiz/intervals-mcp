package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"github.com/teoruiz/intervals-mcp/internal/auth"
	"github.com/teoruiz/intervals-mcp/internal/config"
	"github.com/teoruiz/intervals-mcp/internal/insights"
	"github.com/teoruiz/intervals-mcp/internal/intervals"
	"github.com/teoruiz/intervals-mcp/internal/mcpserver"
	"github.com/teoruiz/intervals-mcp/internal/oauthui"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(".env")
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}
	intervalsClient, err := intervals.NewClient(intervals.Config{
		BaseURL:    cfg.IntervalsBaseURL,
		APIKey:     cfg.IntervalsAPIKey,
		AthleteID:  cfg.IntervalsAthleteID,
		HTTPClient: httpClient,
		Timeout:    cfg.RequestTimeout,
	})
	if err != nil {
		return err
	}

	verifier, err := auth.NewJWTVerifier(auth.VerifierConfig{
		IssuerURL:      cfg.OIDCIssuerURL,
		Audience:       cfg.OIDCAudience,
		AllowedEmail:   cfg.OIDCAllowedEmail,
		AllowedSubject: cfg.OIDCAllowedSub,
		JWKSURL:        cfg.OIDCJWKSURL,
		HTTPClient:     httpClient,
	})
	if err != nil {
		return err
	}

	oauthHandler, err := oauthui.New(oauthui.Config{
		SupabaseURL:     cfg.SupabaseURL,
		SupabaseAnonKey: cfg.SupabaseAnonKey,
		Providers:       cfg.SupabaseOAuthProviders,
	})
	if err != nil {
		return err
	}

	service := insights.New(intervalsClient)
	mcpServer := mcpserver.New(service)
	mcpHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{Stateless: true})
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
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /readyz", readyHandler)

	server := &http.Server{
		Addr:              cfg.MCPAddr,
		Handler:           loggingMiddleware(logger, securityHeaders(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("starting intervals MCP server", "addr", cfg.MCPAddr, "public_url", cfg.MCPPublicURL)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
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

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
