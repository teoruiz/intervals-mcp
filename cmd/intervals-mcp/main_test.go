package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/teoruiz/intervals-mcp/internal/config"
)

func TestParseFlags(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
	}{
		{"defaults", nil, options{}, false},
		{"local", []string{"--local"}, options{Local: true}, false},
		{"addr", []string{"--addr", "127.0.0.1:9000"}, options{Addr: "127.0.0.1:9000"}, false},
		{"env", []string{"--env", "custom.env"}, options{EnvPath: "custom.env", EnvExplicit: true}, false},
		{"combined", []string{"--local", "--addr", "0.0.0.0:8080", "--env", ".env.local"}, options{Local: true, Addr: "0.0.0.0:8080", EnvPath: ".env.local", EnvExplicit: true}, false},
		{"unknown flag", []string{"--nope"}, options{}, true},
		{"positional arg", []string{"serve"}, options{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseFlags(%v) expected error, got %+v", tc.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%v) unexpected error: %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("parseFlags(%v) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestLocalAddr(t *testing.T) {
	t.Run("flag wins over env", func(t *testing.T) {
		if got := localAddr("127.0.0.1:9000", "0.0.0.0:7000", true); got != "127.0.0.1:9000" {
			t.Fatalf("localAddr flag = %q, want 127.0.0.1:9000", got)
		}
	})
	t.Run("env fallback", func(t *testing.T) {
		if got := localAddr("", "0.0.0.0:7000", true); got != "0.0.0.0:7000" {
			t.Fatalf("localAddr env = %q, want 0.0.0.0:7000", got)
		}
	})
	t.Run("ignores dotenv-loaded env", func(t *testing.T) {
		if got := localAddr("", ":8080", false); got != defaultLocalAddr {
			t.Fatalf("localAddr dotenv-loaded env = %q, want %q", got, defaultLocalAddr)
		}
	})
	t.Run("default when unset", func(t *testing.T) {
		if got := localAddr("", "", false); got != defaultLocalAddr {
			t.Fatalf("localAddr default = %q, want %q", got, defaultLocalAddr)
		}
	})
}

func TestIsLoopbackAddr(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:8080", true},
		{"localhost:8080", true},
		{"[::1]:8080", true},
		{"127.0.0.1", true},
		{"localhost", true},
		{"0.0.0.0:8080", false},
		{":8080", false},
		{"192.168.1.5:8080", false},
	}
	for _, tc := range cases {
		if got := isLoopbackAddr(tc.addr); got != tc.want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func testConfig() config.Config {
	return config.Config{
		IntervalsBaseURL:   "https://example.test",
		IntervalsAPIKey:    "test-key",
		IntervalsAthleteID: "i1",
		RequestTimeout:     15 * time.Second,
	}
}

// TestLocalMCPServesToolsWithoutAuth verifies that the handler used by --local
// serves the MCP tool list over HTTP without requiring a bearer token.
func TestLocalMCPServesToolsWithoutAuth(t *testing.T) {
	cfg := testConfig()
	handler, err := buildMCPHandler(cfg, &http.Client{Timeout: cfg.RequestTimeout})
	if err != nil {
		t.Fatalf("buildMCPHandler: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("POST /mcp", handler)
	mux.Handle("GET /mcp", handler)
	mux.Handle("DELETE /mcp", handler)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("connect without auth: %v", err)
	}
	defer func() { _ = session.Close() }()

	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	got := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	want := []string{
		"today_context",
		"list_recent_activities",
		"get_activity",
		"get_recovery",
		"list_calendar",
		"search",
		"fetch",
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing tool %q (got %v)", name, got)
		}
	}
}

// TestProtectedMCPRequiresBearerToken is the authenticated-path counterpart: the
// same handler, wrapped in RequireBearerToken, rejects a tokenless request.
func TestProtectedMCPRequiresBearerToken(t *testing.T) {
	cfg := testConfig()
	handler, err := buildMCPHandler(cfg, &http.Client{Timeout: cfg.RequestTimeout})
	if err != nil {
		t.Fatalf("buildMCPHandler: %v", err)
	}

	verify := func(context.Context, string, *http.Request) (*mcpauth.TokenInfo, error) {
		return nil, mcpauth.ErrInvalidToken
	}
	protected := mcpauth.RequireBearerToken(verify, &mcpauth.RequireBearerTokenOptions{
		ResourceMetadataURL: "https://example.test/.well-known/oauth-protected-resource",
	})(handler)

	mux := http.NewServeMux()
	mux.Handle("POST /mcp", protected)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Errorf("expected WWW-Authenticate header on 401 response")
	}
}
