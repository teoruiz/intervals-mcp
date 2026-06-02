package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
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

const defaultLocalAddr = "127.0.0.1:8080"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger, os.Args[1:]); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

type options struct {
	Local   bool
	Addr    string
	EnvPath string
}

func parseFlags(args []string) (options, error) {
	fs := flag.NewFlagSet("intervals-mcp", flag.ContinueOnError)
	opts := options{EnvPath: ".env"}
	fs.BoolVar(&opts.Local, "local", false, "run an unauthenticated MCP server for local use (no Supabase/OIDC)")
	fs.StringVar(&opts.Addr, "addr", "", "override the listen address (local mode defaults to "+defaultLocalAddr+", otherwise MCP_ADDR)")
	fs.StringVar(&opts.EnvPath, "env", ".env", "path to the dotenv file to load")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return opts, nil
}

func run(logger *slog.Logger, args []string) error {
	opts, err := parseFlags(args)
	if err != nil {
		return err
	}
	if opts.Local {
		return serveLocal(logger, opts)
	}
	return serveAuthenticated(logger, opts)
}

func serveAuthenticated(logger *slog.Logger, opts options) error {
	cfg, err := config.Load(opts.EnvPath)
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}
	mcpHandler, err := buildMCPHandler(cfg, httpClient)
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

	logger.Info("starting intervals MCP server", "addr", cfg.MCPAddr, "public_url", cfg.MCPPublicURL)
	return runHTTPServer(server, cfg.ShutdownTimeout)
}

func serveLocal(logger *slog.Logger, opts options) error {
	envAddr, envAddrSet := os.LookupEnv("MCP_ADDR")

	cfg, err := config.LoadIntervals(opts.EnvPath)
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}
	mcpHandler, err := buildMCPHandler(cfg, httpClient)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("POST /mcp", mcpHandler)
	mux.Handle("GET /mcp", mcpHandler)
	mux.Handle("DELETE /mcp", mcpHandler)
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /readyz", readyHandler)

	addr := localAddr(opts.Addr, envAddr, envAddrSet)
	server := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(logger, securityHeaders(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Warn("starting local intervals MCP server with authentication disabled", "addr", addr, "endpoint", "http://"+addr+"/mcp")
	if !isLoopbackAddr(addr) {
		logger.Warn("local MCP server is unauthenticated and not bound to loopback; anyone who can reach this address has full read access to the configured Intervals.icu athlete", "addr", addr)
	}
	return runHTTPServer(server, cfg.ShutdownTimeout)
}

func buildMCPHandler(cfg config.Config, httpClient *http.Client) (http.Handler, error) {
	intervalsClient, err := intervals.NewClient(intervals.Config{
		BaseURL:    cfg.IntervalsBaseURL,
		APIKey:     cfg.IntervalsAPIKey,
		AthleteID:  cfg.IntervalsAthleteID,
		HTTPClient: httpClient,
		Timeout:    cfg.RequestTimeout,
	})
	if err != nil {
		return nil, err
	}
	service := insights.New(intervalsClient)
	mcpServer := mcpserver.New(service)
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{Stateless: true}), nil
}

func runHTTPServer(server *http.Server, shutdownTimeout time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func localAddr(flagAddr, envAddr string, envAddrSet bool) string {
	if strings.TrimSpace(flagAddr) != "" {
		return flagAddr
	}
	if envAddrSet && strings.TrimSpace(envAddr) != "" {
		return envAddr
	}
	return defaultLocalAddr
}

func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
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
