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

	"github.com/teoruiz/intervals-mcp/internal/config"
	appruntime "github.com/teoruiz/intervals-mcp/internal/runtime"
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
	Addr        string
	EnvPath     string
	EnvExplicit bool
}

func newFlagSet(opts *options) *flag.FlagSet {
	fs := flag.NewFlagSet("intervals-mcp", flag.ContinueOnError)
	fs.StringVar(&opts.Addr, "addr", "", "override the listen address (defaults to "+defaultLocalAddr+", or process-env MCP_ADDR)")
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
	return serve(logger, opts)
}

// serve runs the MCP server without authentication. Remote deployments put it
// behind the Cloudflare Worker, which terminates OAuth and proxies only
// authorized traffic; locally it is meant to stay on loopback.
func serve(logger *slog.Logger, opts options) error {
	envAddr, envAddrSet := os.LookupEnv("MCP_ADDR")

	cfg, _, err := config.LoadIntervalsDiscovered(config.DiscoveryOptions{
		EnvPath:     opts.EnvPath,
		EnvExplicit: opts.EnvExplicit,
	})
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

	logger.Info("starting intervals MCP server", "addr", addr, "endpoint", "http://"+addr+"/mcp")
	if !isLoopbackAddr(addr) {
		logger.Info("MCP server is unauthenticated and not bound to loopback; expected inside the Cloudflare container, otherwise anyone who can reach this address has full read access to the configured Intervals.icu athlete", "addr", addr)
	}
	return runHTTPServer(server, cfg.ShutdownTimeout)
}

func buildMCPHandler(cfg config.Config, httpClient *http.Client) (http.Handler, error) {
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
			"user", r.Header.Get("X-Intervals-Authenticated-User"),
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
