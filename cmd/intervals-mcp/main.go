package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	appruntime "github.com/teoruiz/intervals-mcp/internal/platform/runtime"
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
	Local       bool
	Addr        string
	EnvPath     string
	EnvExplicit bool
}

func newFlagSet(opts *options) *flag.FlagSet {
	fs := flag.NewFlagSet("intervals-mcp", flag.ContinueOnError)
	fs.BoolVar(&opts.Local, "local", false, "run an unauthenticated MCP server for local use (no Supabase/OIDC)")
	fs.StringVar(&opts.Addr, "addr", "", "override the listen address (local mode defaults to "+defaultLocalAddr+", otherwise MCP_ADDR)")
	fs.StringVar(&opts.EnvPath, "env", "", "path to the dotenv file to load instead of discovered config")
	return fs
}

func parseFlags(args []string) (options, error) {
	var opts options
	fs := newFlagSet(&opts)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "env" {
			opts.EnvExplicit = true
		}
	})
	if fs.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return opts, nil
}

func run(logger *slog.Logger, args []string) error {
	opts, err := parseFlags(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			var help options
			fs := newFlagSet(&help)
			fs.SetOutput(os.Stdout)
			fs.Usage()
			return nil
		}
		return err
	}
	if opts.Local {
		return serveLocal(logger, opts)
	}
	return serveAuthenticated(logger, opts)
}

func serveAuthenticated(logger *slog.Logger, opts options) error {
	cfg, _, err := appruntime.LoadDiscovered(appruntime.DiscoveryOptions{
		EnvPath:     opts.EnvPath,
		EnvExplicit: opts.EnvExplicit,
	})
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}
	handler, err := appruntime.NewAuthenticatedHTTPHandler(cfg, httpClient)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.MCPAddr,
		Handler:           loggingMiddleware(logger, securityHeaders(handler)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("starting intervals MCP server", "addr", cfg.MCPAddr, "public_url", cfg.MCPPublicURL)
	return runHTTPServer(server, cfg.ShutdownTimeout)
}

func serveLocal(logger *slog.Logger, opts options) error {
	envAddr, envAddrSet := os.LookupEnv("MCP_ADDR")

	cfg, _, err := appruntime.LoadIntervalsDiscovered(appruntime.DiscoveryOptions{
		EnvPath:     opts.EnvPath,
		EnvExplicit: opts.EnvExplicit,
	})
	if err != nil {
		return err
	}

	httpClient := &http.Client{Timeout: cfg.RequestTimeout}
	handler, err := appruntime.NewLocalHTTPHandler(cfg, httpClient)
	if err != nil {
		return err
	}

	addr := localAddr(opts.Addr, envAddr, envAddrSet)
	server := &http.Server{
		Addr:              addr,
		Handler:           loggingMiddleware(logger, securityHeaders(handler)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Warn("starting local intervals MCP server with authentication disabled", "addr", addr, "endpoint", "http://"+addr+"/mcp")
	if !isLoopbackAddr(addr) {
		logger.Warn("local MCP server is unauthenticated and not bound to loopback; anyone who can reach this address has full read access to the configured Intervals.icu athlete", "addr", addr)
	}
	return runHTTPServer(server, cfg.ShutdownTimeout)
}

func buildMCPHandler(cfg appruntime.Config, httpClient *http.Client) (http.Handler, error) {
	return appruntime.NewMCPHTTPHandler(cfg, httpClient)
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
